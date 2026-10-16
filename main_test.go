package main

import "testing"

func TestNeedsReviewDecision(t *testing.T) {
	tests := []struct {
		name     string
		shipment Shipment
		want     bool
	}{
		{"exception escalates", Shipment{Events: []ShipmentEvent{{Status: "exception"}}, ProofFile: "pod.pdf"}, true},
		{"complete delivery stays automatic", Shipment{Events: []ShipmentEvent{{Status: "delivered"}}, ProofFile: "pod.pdf"}, false},
		{"missing proof escalates", Shipment{Events: []ShipmentEvent{{Status: "delivered"}}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsReview(tc.shipment); got != tc.want {
				t.Fatalf("needsReview()=%v, want %v", got, tc.want)
			}
		})
	}
}
