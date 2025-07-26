"use client";

import React, { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Card, 
    CardContent, 
    CardHeader, 
    CardTitle, 
    CardDescription } from "@/components/ui/card";
import { AggregatorCandidate } from "@/types/federated-learning"; // 구체화된 타입 임포트

interface Props {
	candidates: AggregatorCandidate[];
	onSelect?: (selectedId: string) => void;
}

const AggregatorSelection = ({ candidates }: Props) => {
	console.log("AggregatorSelection rendered with candidates:", candidates);
	return (
		<div className="space-y-4 p-1">
			{candidates.map((candidate) => (
				<Card key={candidate.id} className="transition-all hover:shadow-lg">
					<CardHeader>
						<div className="flex justify-between items-start">
							<div>
								<CardTitle>{candidate.name}</CardTitle>
								<CardDescription>
									알고리즘: <span className="font-medium text-primary">{candidate.algorithm}</span>
								</CardDescription>
							</div>
							<Button>
								이 집계자로 선택
							</Button>
						</div>
					</CardHeader>
					<CardContent>
						<div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
							<div className="p-3 bg-muted rounded-lg">
								<p className="text-muted-foreground">인스턴스 타입</p>
								<p className="font-semibold">{candidate.intstance_type}</p>
							</div>
							<div className="p-3 bg-muted rounded-lg">
								<p className="text-muted-foreground">예상 정확도</p>
								<p className="font-semibold">{candidate.expected_accuracy?.toFixed(2) ?? 'N/A'}%</p>
							</div>
							<div className="p-3 bg-muted rounded-lg">
								<p className="text-muted-foreground">예상 비용</p>
								<p className="font-semibold">${candidate.estimated_cost.toFixed(2)}</p>
							</div>
							<div className="p-3 bg-muted rounded-lg">
								<p className="text-muted-foreground">CPU SPEC</p>
								<p className="font-semibold truncate">{candidate.cpuSpecs}</p>
							</div>
							<div className="p-3 bg-muted rounded-lg">
								<p className="text-muted-foreground">Memory SPEC</p>
								<p className="font-semibold truncate">{candidate.memorySpecs}</p>
							</div>
						</div>
					</CardContent>
				</Card>
			))}
		</div>
	);
};

export default AggregatorSelection;