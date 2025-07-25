package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Mungge/Fleecy-Cloud/models"
)

// OpenStack 인증 토큰 응답
type AuthTokenResponse struct {
	Token struct {
		ID        string    `json:"id"`
		ExpiresAt time.Time `json:"expires_at"`
		Project   struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"project"`
	} `json:"token"`
}

// OpenStack 인증 요청
type AuthRequest struct {
	Auth struct {
		Identity struct {
			Methods               []string `json:"methods"`
			ApplicationCredential *struct {
				ID     string `json:"id"`
				Secret string `json:"secret"`
			} `json:"application_credential,omitempty"`
		} `json:"identity"`
	} `json:"auth"`
}

// Flavor 상세 정보
type FlavorDetails struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	VCPUs int    `json:"vcpus"`
	RAM   int    `json:"ram"`   // MB 단위
	Disk  int    `json:"disk"`  // GB 단위
}

// VM 인스턴스 정보
type VMInstance struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Flavor   FlavorDetails `json:"flavor"`
	Addresses map[string][]struct {
		Addr    string `json:"addr"`
		Type    string `json:"OS-EXT-IPS:type"`
	} `json:"addresses"`
	PowerState       int    `json:"OS-EXT-STS:power_state"`
	AvailabilityZone string `json:"OS-EXT-AZ:availability_zone"`
	Created          string `json:"created"`
	Updated          string `json:"updated"`
}

// VM 목록 조회 응답
type VMListResponse struct {
	Servers []VMInstance `json:"servers"`
}

// VM 모니터링 정보
type VMMonitoringInfo struct {
	InstanceID      string    `json:"instance_id"`
	Status          string    `json:"status"`
	CPUUsage        float64   `json:"cpu_usage"`
	MemoryUsage     float64   `json:"memory_usage"`
	DiskUsage       float64   `json:"disk_usage"`
	NetworkInBytes  int64     `json:"network_in_bytes"`
	NetworkOutBytes int64     `json:"network_out_bytes"`
	LastUpdated     time.Time `json:"last_updated"`
}

// VM 헬스체크 결과
type VMHealthCheckResult struct {
	Healthy     bool      `json:"healthy"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	CheckedAt   time.Time `json:"checked_at"`
	ResponseTime int64    `json:"response_time_ms"`
}

type OpenStackService struct {
	client *http.Client
}

func NewOpenStackService() *OpenStackService {
	return &OpenStackService{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// OpenStack 인증 토큰 획득 -> TestConnection
func (s *OpenStackService) GetAuthToken(participant *models.Participant) (string, error) {
	authReq := AuthRequest{}
	
	// Application Credential 방식만 지원
	if participant.OpenStackApplicationCredentialID != "" && participant.OpenStackApplicationCredentialSecret != "" {
		// Application Credential 방식
		authReq.Auth.Identity.Methods = []string{"application_credential"}
		authReq.Auth.Identity.ApplicationCredential = &struct {
			ID     string `json:"id"`
			Secret string `json:"secret"`
		}{
			ID:     participant.OpenStackApplicationCredentialID,
			Secret: participant.OpenStackApplicationCredentialSecret,
		}
	} else {
		return "", fmt.Errorf("application Credential 인증 정보가 필요합니다")
	}

	jsonData, err := json.Marshal(authReq)
	if err != nil {
		return "", fmt.Errorf("인증 요청 생성 실패: %v", err)
	}

	url := fmt.Sprintf("%s/identity/v3/auth/tokens", participant.OpenStackEndpoint)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("HTTP 요청 생성 실패: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("인증 요청 실패: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("인증 실패: HTTP %d", resp.StatusCode)
	}

	token := resp.Header.Get("X-Subject-Token")
	if token == "" {
		return "", fmt.Errorf("인증 토큰을 받지 못했습니다")
	}

	return token, nil
}

func (s *OpenStackService) GetAllVMInstances(participant *models.Participant) ([]VMInstance, error) {
    token, err := s.GetAuthToken(participant)
    if err != nil {
        return nil, fmt.Errorf("인증 실패: %v", err)
    }

	url := fmt.Sprintf("%s/compute/v2.1/servers/detail", participant.OpenStackEndpoint)
    
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("HTTP 요청 생성 실패: %v", err)
    }

    req.Header.Set("X-Auth-Token", token)
    req.Header.Set("Accept", "application/json")

    resp, err := s.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("VM 목록 조회 실패: %v", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("응답 읽기 실패: %v", err)
    }

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("VM 목록 조회 실패: HTTP %d, 응답: %s", resp.StatusCode, string(body))
    }

    // 먼저 기본 VM 정보를 파싱
    var basicResponse struct {
        Servers []struct {
            ID       string `json:"id"`
            Name     string `json:"name"`
            Status   string `json:"status"`
            Flavor   struct {
                ID string `json:"id"`
            } `json:"flavor"`
            Addresses map[string][]struct {
                Addr    string `json:"addr"`
                Type    string `json:"OS-EXT-IPS:type"`
            } `json:"addresses"`
            PowerState       int    `json:"OS-EXT-STS:power_state"`
            AvailabilityZone string `json:"OS-EXT-AZ:availability_zone"`
            Created          string `json:"created"`
            Updated          string `json:"updated"`
        } `json:"servers"`
    }

    if err := json.Unmarshal(body, &basicResponse); err != nil {
        return nil, fmt.Errorf("응답 파싱 실패: %v, 응답 내용: %s", err, string(body))
    }

    // 각 VM에 대해 flavor 상세 정보를 가져와서 완전한 VMInstance 생성
    var vmInstances []VMInstance
    for _, server := range basicResponse.Servers {
        flavorDetails, err := s.GetFlavorDetails(participant, token, server.Flavor.ID)
        if err != nil {
            // Flavor 정보를 가져오지 못한 경우 기본값으로 설정
            flavorDetails = &FlavorDetails{
                ID:    server.Flavor.ID,
                Name:  "Unknown",
                VCPUs: 0,
                RAM:   0,
                Disk:  0,
            }
        }

        vmInstance := VMInstance{
            ID:               server.ID,
            Name:             server.Name,
            Status:           server.Status,
            Flavor:           *flavorDetails,
            Addresses:        server.Addresses,
            PowerState:       server.PowerState,
            Created:          server.Created,
            Updated:          server.Updated,
        }

        vmInstances = append(vmInstances, vmInstance)
    }

    return vmInstances, nil
}


// VM 인스턴스 정보 조회
func (s *OpenStackService) GetVMInstance(vm *models.VirtualMachine, participant *models.Participant, token string) (*VMInstance, error) {
	if vm.InstanceID == "" {
		return nil, fmt.Errorf("VM 인스턴스 ID가 설정되지 않았습니다")
	}

	url := fmt.Sprintf("%s/compute/v2.1/servers/%s", participant.OpenStackEndpoint, vm.InstanceID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("HTTP 요청 생성 실패: %v", err)
	}

	req.Header.Set("X-Auth-Token", token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("VM 정보 조회 실패: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("VM 정보 조회 실패: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("응답 읽기 실패: %v", err)
	}

	var basicResponse struct {
		Server struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Status   string `json:"status"`
			Flavor   struct {
				ID string `json:"id"`
			} `json:"flavor"`
			Addresses map[string][]struct {
				Addr    string `json:"addr"`
				Type    string `json:"OS-EXT-IPS:type"`
			} `json:"addresses"`
			PowerState       int    `json:"OS-EXT-STS:power_state"`
			AvailabilityZone string `json:"OS-EXT-AZ:availability_zone"`
			Created          string `json:"created"`
			Updated          string `json:"updated"`
		} `json:"server"`
	}

	if err := json.Unmarshal(body, &basicResponse); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %v", err)
	}

	// Flavor 상세 정보 조회
	flavorDetails, err := s.GetFlavorDetails(participant, token, basicResponse.Server.Flavor.ID)
	if err != nil {
		// Flavor 정보를 가져오지 못한 경우 기본값으로 설정
		flavorDetails = &FlavorDetails{
			ID:    basicResponse.Server.Flavor.ID,
			Name:  "Unknown",
			VCPUs: 0,
			RAM:   0,
			Disk:  0,
		}
	}

	vmInstance := &VMInstance{
		ID:               basicResponse.Server.ID,
		Name:             basicResponse.Server.Name,
		Status:           basicResponse.Server.Status,
		Flavor:           *flavorDetails,
		Addresses:        basicResponse.Server.Addresses,
		PowerState:       basicResponse.Server.PowerState,
		Created:          basicResponse.Server.Created,
		Updated:          basicResponse.Server.Updated,
	}

	return vmInstance, nil
}

// VM 목록 조회
func (s *OpenStackService) ListVMInstances(participant *models.Participant, token string) ([]VMInstance, error) {
	url := fmt.Sprintf("%s/v2.1/servers/detail", participant.OpenStackEndpoint)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("HTTP 요청 생성 실패: %v", err)
	}

	req.Header.Set("X-Auth-Token", token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("VM 목록 조회 실패: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("VM 목록 조회 실패: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("응답 읽기 실패: %v", err)
	}

	var response VMListResponse

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %v", err)
	}

	return response.Servers, nil
}

// MonitorSpecificVM은 특정 VM의 모니터링 정보를 조회합니다 (더 이상 DB에 저장하지 않음)
func (s *OpenStackService) MonitorSpecificVM(participant *models.Participant, vm *models.VirtualMachine) (*models.VMMonitoringInfo, error) {
	// 실제 환경에서는 OpenStack의 telemetry 서비스(Ceilometer)나 
	// Prometheus 메트릭을 통해 실제 모니터링 데이터를 수집해야 합니다.
	// 여기서는 시뮬레이션 데이터를 반환합니다.
	return s.GetVMMonitoringInfo(vm.InstanceID)
}

// VM 헬스체크 수행
func (s *OpenStackService) HealthCheckSpecificVM(participant *models.Participant, vm *models.VirtualMachine) (*VMHealthCheckResult, error) {
	startTime := time.Now()
	
	token, err := s.GetAuthToken(participant)
	if err != nil {
		return &VMHealthCheckResult{
			Healthy:      false,
			Status:       "ERROR",
			Message:      fmt.Sprintf("인증 실패: %v", err),
			CheckedAt:    time.Now(),
			ResponseTime: time.Since(startTime).Milliseconds(),
		}, nil
	}

	instance, err := s.GetVMInstance(vm, participant, token)
	if err != nil {
		return &VMHealthCheckResult{
			Healthy:      false,
			Status:       "ERROR",
			Message:      fmt.Sprintf("VM 조회 실패: %v", err),
			CheckedAt:    time.Now(),
			ResponseTime: time.Since(startTime).Milliseconds(),
		}, nil
	}

	healthy := instance.Status == "ACTIVE"
	status := instance.Status
	message := "VM이 정상적으로 동작 중입니다"
	
	if !healthy {
		message = fmt.Sprintf("VM 상태가 비정상입니다: %s", instance.Status)
	}

	return &VMHealthCheckResult{
		Healthy:      healthy,
		Status:       status,
		Message:      message,
		CheckedAt:    time.Now(),
		ResponseTime: time.Since(startTime).Milliseconds(),
	}, nil
}

// 연합학습 작업 할당 (특정 VirtualMachine 인스턴스 기반)
func (s *OpenStackService) AssignFederatedLearningTaskSpecific(participant *models.Participant, vm *models.VirtualMachine, taskID string) error {
	// 현재 VM 상태 확인
	token, err := s.GetAuthToken(participant)
	if err != nil {
		return fmt.Errorf("인증 실패: %v", err)
	}

	instance, err := s.GetVMInstance(vm, participant, token)
	if err != nil {
		return fmt.Errorf("VM 상태 확인 실패: %v", err)
	}

	if instance.Status != "ACTIVE" {
		return fmt.Errorf("VM이 활성 상태가 아닙니다: %s", instance.Status)
	}

	// 실제 환경에서는 VM에 SSH 연결하거나 에이전트를 통해 
	// 연합학습 작업을 할당하고 실행합니다.
	// 여기서는 시뮬레이션합니다.
	
	return nil
}

// GetFlavorDetails는 특정 flavor의 상세 정보를 조회합니다
func (s *OpenStackService) GetFlavorDetails(participant *models.Participant, token string, flavorID string) (*FlavorDetails, error) {
	url := fmt.Sprintf("%s/compute/v2.1/flavors/%s", participant.OpenStackEndpoint, flavorID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("HTTP 요청 생성 실패: %v", err)
	}

	req.Header.Set("X-Auth-Token", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Flavor 정보 조회 실패: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Flavor 정보 조회 실패: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("응답 읽기 실패: %v", err)
	}

	var response struct {
		Flavor FlavorDetails `json:"flavor"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %v", err)
	}

	return &response.Flavor, nil
}

// SyncVMsFromOpenStack은 OpenStack에서 VM 정보를 동기화하여 DB에 저장합니다
func (s *OpenStackService) SyncVMsFromOpenStack(participant *models.Participant) ([]models.VirtualMachine, error) {
	openStackVMs, err := s.GetAllVMInstances(participant)
	if err != nil {
		return nil, fmt.Errorf("OpenStack VM 목록 조회 실패: %v", err)
	}

	var syncedVMs []models.VirtualMachine
	
	for _, osVM := range openStackVMs {
		// IP 주소 직렬화
		ipAddressesJSON, _ := json.Marshal(osVM.Addresses)
		
		// VM 정보 구성 (DB에 저장할 안정적인 정보만)
		vm := models.VirtualMachine{
			InstanceID:       osVM.ID,
			Name:            osVM.Name,
			ParticipantID:   participant.ID,
			Status:          osVM.Status,
			FlavorID:        osVM.Flavor.ID,
			FlavorName:      osVM.Flavor.Name,
			VCPUs:          osVM.Flavor.VCPUs,
			RAM:            osVM.Flavor.RAM,
			Disk:           osVM.Flavor.Disk,
			IPAddresses:    string(ipAddressesJSON),
			AvailabilityZone: osVM.AvailabilityZone,
		}
		
		syncedVMs = append(syncedVMs, vm)
	}
	
	return syncedVMs, nil
}

// GetVMRuntimeStatus는 실시간 VM 상태를 조회합니다 (DB에 저장하지 않음)
func (s *OpenStackService) GetVMRuntimeStatus(participant *models.Participant, instanceID string) (*models.VMRuntimeInfo, error) {
	token, err := s.GetAuthToken(participant)
	if err != nil {
		return nil, fmt.Errorf("인증 실패: %v", err)
	}

	url := fmt.Sprintf("%s/compute/v2.1/servers/%s", participant.OpenStackEndpoint, instanceID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("HTTP 요청 생성 실패: %v", err)
	}

	req.Header.Set("X-Auth-Token", token)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("VM 상태 조회 실패: %v", err)
	}
	defer resp.Body.Close()

	var response struct {
		Server struct {
			Status     string `json:"status"`
			PowerState int    `json:"OS-EXT-STS:power_state"`
		} `json:"server"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %v", err)
	}

	return &models.VMRuntimeInfo{
		InstanceID:  instanceID,
		Status:      response.Server.Status,
		PowerState:  response.Server.PowerState,
		LastChecked: time.Now(),
	}, nil
}

// GetVMMonitoringInfo는 모니터링 정보를 조회합니다 (시뮬레이션)
func (s *OpenStackService) GetVMMonitoringInfo(instanceID string) (*models.VMMonitoringInfo, error) {
	// 실제 환경에서는 Ceilometer, Prometheus 등에서 데이터 수집
	// 현재는 시뮬레이션 데이터 반환
	return &models.VMMonitoringInfo{
		InstanceID:      instanceID,
		CPUUsage:        75.5,
		MemoryUsage:     82.3,
		DiskUsage:       45.8,
		NetworkInBytes:  1024000,
		NetworkOutBytes: 2048000,
		LastUpdated:     time.Now(),
	}, nil
}
