package main

import "testing"

func TestSecurityRulePayloadIngressTCPPorts(t *testing.T) {
	minPort := 80
	maxPort := 443
	payload, err := securityRulePayload(OCISecurityRuleInput{
		Protocol:    "tcp",
		Source:      "0.0.0.0/0",
		SourceType:  "CIDR_BLOCK",
		MinPort:     &minPort,
		MaxPort:     &maxPort,
		Description: "web",
	})
	if err != nil {
		t.Fatalf("securityRulePayload returned error: %v", err)
	}

	if payload["protocol"] != "6" || payload["source"] != "0.0.0.0/0" || payload["sourceType"] != "CIDR_BLOCK" {
		t.Fatalf("unexpected ingress payload: %+v", payload)
	}

	tcpOptions, ok := payload["tcpOptions"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tcpOptions in payload: %+v", payload)
	}
	rangeValue, ok := tcpOptions["destinationPortRange"].(map[string]interface{})
	if !ok || rangeValue["min"] != 80 || rangeValue["max"] != 443 {
		t.Fatalf("unexpected destination port range: %+v", tcpOptions)
	}
}

func TestSecurityRulePayloadEgressDestination(t *testing.T) {
	payload, err := securityRulePayload(OCISecurityRuleInput{
		Protocol:      "all",
		Destination:   "0.0.0.0/0",
		DestType:      "CIDR_BLOCK",
		IsDestination: true,
	})
	if err != nil {
		t.Fatalf("securityRulePayload returned error: %v", err)
	}

	if payload["protocol"] != "all" || payload["destination"] != "0.0.0.0/0" || payload["destinationType"] != "CIDR_BLOCK" {
		t.Fatalf("unexpected egress payload: %+v", payload)
	}
	if _, ok := payload["source"]; ok {
		t.Fatalf("did not expect source fields for egress: %+v", payload)
	}
}

func TestSecurityRulePayloadICMPOmitsPorts(t *testing.T) {
	icmpType := 3
	icmpCode := 4
	payload, err := securityRulePayload(OCISecurityRuleInput{
		Protocol:   "icmp",
		Source:     "10.0.0.0/24",
		SourceType: "CIDR_BLOCK",
		ICMPType:   &icmpType,
		ICMPCode:   &icmpCode,
	})
	if err != nil {
		t.Fatalf("securityRulePayload returned error: %v", err)
	}

	if payload["protocol"] != "1" {
		t.Fatalf("expected ICMP protocol 1, got %+v", payload)
	}
	if _, ok := payload["tcpOptions"]; ok {
		t.Fatalf("did not expect tcpOptions for ICMP: %+v", payload)
	}
	if _, ok := payload["udpOptions"]; ok {
		t.Fatalf("did not expect udpOptions for ICMP: %+v", payload)
	}
	icmpOptions, ok := payload["icmpOptions"].(map[string]interface{})
	if !ok || icmpOptions["type"] != 3 || icmpOptions["code"] != 4 {
		t.Fatalf("unexpected ICMP options: %+v", payload)
	}
}

func TestSecurityRulePayloadRejectsInvalidPortRange(t *testing.T) {
	minPort := 9000
	maxPort := 80
	_, err := securityRulePayload(OCISecurityRuleInput{
		Protocol:   "udp",
		Source:     "0.0.0.0/0",
		SourceType: "CIDR_BLOCK",
		MinPort:    &minPort,
		MaxPort:    &maxPort,
	})
	if err == nil {
		t.Fatal("expected invalid port range to return an error")
	}
}

func TestSecurityRulePayloadRejectsUnsupportedDestinationType(t *testing.T) {
	_, err := securityRulePayload(OCISecurityRuleInput{
		Protocol:      "all",
		Destination:   "0.0.0.0/0",
		DestType:      "UNSUPPORTED_TYPE",
		IsDestination: true,
	})
	if err == nil {
		t.Fatal("expected unsupported destination type to return an error")
	}
}

func TestSecurityRulePayloadAllowsNetworkSecurityGroupDestinationType(t *testing.T) {
	payload, err := securityRulePayload(OCISecurityRuleInput{
		Protocol:      "all",
		Destination:   "ocid1.networksecuritygroup.oc1.example",
		DestType:      "NETWORK_SECURITY_GROUP",
		IsDestination: true,
	})
	if err != nil {
		t.Fatalf("securityRulePayload returned error: %v", err)
	}
	if payload["destinationType"] != "NETWORK_SECURITY_GROUP" {
		t.Fatalf("unexpected destination type: %+v", payload)
	}
}

func TestSecurityRulePayloadAllowsIANAProtocolNumber(t *testing.T) {
	payload, err := securityRulePayload(OCISecurityRuleInput{
		Protocol:   "50",
		Source:     "0.0.0.0/0",
		SourceType: "CIDR_BLOCK",
	})
	if err != nil {
		t.Fatalf("securityRulePayload returned error: %v", err)
	}
	if payload["protocol"] != "50" {
		t.Fatalf("unexpected protocol: %+v", payload)
	}
}

func TestSecurityRulePayloadRejectsICMPCodeWithoutType(t *testing.T) {
	icmpCode := 4
	_, err := securityRulePayload(OCISecurityRuleInput{
		Protocol:   "icmp",
		Source:     "10.0.0.0/24",
		SourceType: "CIDR_BLOCK",
		ICMPCode:   &icmpCode,
	})
	if err == nil {
		t.Fatal("expected ICMP code without type to return an error")
	}
}
