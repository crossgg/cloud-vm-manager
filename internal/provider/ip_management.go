package provider

type PublicIPInfo struct {
	ID             string `json:"id"`
	IPAddress      string `json:"ipAddress"`
	Lifetime       string `json:"lifetime"`
	DisplayName    string `json:"displayName,omitempty"`
	SKU            string `json:"sku,omitempty"`
	IdleFeeWarning bool   `json:"idleFeeWarning,omitempty"`
}

type PrivateIPInfo struct {
	ID            string        `json:"id"`
	IPAddress     string        `json:"ipAddress"`
	IsPrimary     bool          `json:"isPrimary"`
	HostnameLabel string        `json:"hostnameLabel,omitempty"`
	PublicIP      *PublicIPInfo `json:"publicIp,omitempty"`
}

type IPv6Info struct {
	ID               string `json:"id"`
	IPAddress        string `json:"ipAddress"`
	CIDRPrefixLength int    `json:"cidrPrefixLength"`
	SubnetID         string `json:"subnetId,omitempty"`
	VNICID           string `json:"vnicId,omitempty"`
	CanChange        bool   `json:"canChange"`
	DisplayName      string `json:"displayName,omitempty"`
	Lifetime         string `json:"lifetime,omitempty"`
	SKU              string `json:"sku,omitempty"`
}

type SubnetIPv6Status struct {
	VCNHasIPv6                bool     `json:"vcnHasIpv6"`
	VCNIPv6CIDRs              []string `json:"vcnIpv6Cidrs"`
	SubnetHasIPv6             bool     `json:"subnetHasIpv6"`
	SuggestedHex              string   `json:"suggestedHex"`
	UsedHexes                 []string `json:"usedHexes"`
	RouteHasIPv6Default       bool     `json:"routeHasIpv6Default"`
	HasInternetGateway        bool     `json:"hasInternetGateway"`
	SecurityListHasIPv6Egress bool     `json:"securityListHasIpv6Egress"`

	CanEnable             bool   `json:"canEnable"`
	BlockingReason        string `json:"blockingReason,omitempty"`
	RequiresConfirmation  bool   `json:"requiresConfirmation,omitempty"`
	ConfirmationMessage   string `json:"confirmationMessage,omitempty"`
	IsAutoModeVPC         bool   `json:"isAutoModeVpc,omitempty"`
	HasBasicSKUConflict   bool   `json:"hasBasicSkuConflict,omitempty"`
	NeedsFirewallIPv6Rule bool   `json:"needsFirewallIpv6Rule,omitempty"`
	FirewallHint          string `json:"firewallHint,omitempty"`
}

type VNICIPInfo struct {
	ID                    string            `json:"id"`
	DisplayName           string            `json:"displayName"`
	IsPrimary             bool              `json:"isPrimary"`
	LifecycleState        string            `json:"lifecycleState"`
	MACAddress            string            `json:"macAddress,omitempty"`
	SubnetID              string            `json:"subnetId,omitempty"`
	SubnetName            string            `json:"subnetName,omitempty"`
	VCNID                 string            `json:"vcnId,omitempty"`
	IPv4CIDR              string            `json:"ipv4Cidr,omitempty"`
	IPv6CIDRBlocks        []string          `json:"ipv6CidrBlocks"`
	IPv6Setup             *SubnetIPv6Status `json:"ipv6Setup,omitempty"`
	ProhibitPublicIPOnNIC bool              `json:"prohibitPublicIpOnVnic"`
	PrivateIPs            []PrivateIPInfo   `json:"privateIps"`
	IPv6s                 []IPv6Info        `json:"ipv6s"`
}

type ReservedPublicIP struct {
	ID          string `json:"id"`
	IPAddress   string `json:"ipAddress"`
	DisplayName string `json:"displayName,omitempty"`
	PrivateIPID string `json:"privateIpId,omitempty"`
	SKU         string `json:"sku,omitempty"`
	Zone        string `json:"zone,omitempty"`
}

type DNSBindingSummary struct {
	HasAny  bool `json:"hasAny"`
	HasA    bool `json:"hasA"`
	HasAAAA bool `json:"hasAAAA"`
}

type IPCapabilities struct {
	CanAddSecondaryPrivateIPv4 bool     `json:"canAddSecondaryPrivateIpv4"`
	CanAttachReservedIPv4      bool     `json:"canAttachReservedIpv4"`
	CanCreateReservedIPv4      bool     `json:"canCreateReservedIpv4"`
	CanDetachPublicIPv4        bool     `json:"canDetachPublicIpv4"`
	CanAttachIPv6              bool     `json:"canAttachIpv6"`
	CanChangeIPv6              bool     `json:"canChangeIpv6"`
	CanDeleteIPv6              bool     `json:"canDeleteIpv6"`
	CanEnableSubnetIPv6        bool     `json:"canEnableSubnetIpv6"`
	IPv6PrefixConfigurable     bool     `json:"ipv6PrefixConfigurable"`
	HasIdlePublicIPFeeWarning  bool     `json:"hasIdlePublicIpFeeWarning"`
	Notes                      []string `json:"notes,omitempty"`
}

type IPManagementInfo struct {
	Provider     string             `json:"provider"`
	InstanceID   string             `json:"instanceId"`
	InstanceName string             `json:"instanceName"`
	Capabilities IPCapabilities     `json:"capabilities"`
	VNICs        []VNICIPInfo       `json:"vnics"`
	ReservedIPs  []ReservedPublicIP `json:"reservedIps"`
	DNSBindings  DNSBindingSummary  `json:"dnsBindings"`
}

type AttachIPv4Request struct {
	VNICID         string `json:"vnicId"`
	PrivateIPID    string `json:"privateIpId"`
	Mode           string `json:"mode"` // "ephemeral_public" | "secondary_private"
	IPAddress      string `json:"ipAddress,omitempty"`
	HostnameLabel  string `json:"hostnameLabel,omitempty"`
	AssignPublicIP bool   `json:"assignPublicIp"`
}

type DeleteIPv4Request struct {
	VNICID          string `json:"vnicId,omitempty"`
	PrivateIPID     string `json:"privateIpId"`
	PublicIPID      string `json:"publicIpId"`
	DeletePrivateIP bool   `json:"deletePrivateIp"`
	DeletePublicIP  *bool  `json:"deletePublicIp,omitempty"`
}

type AttachReservedIPv4Request struct {
	VNICID             string `json:"vnicId,omitempty"`
	PrivateIPID        string `json:"privateIpId"`
	ReservedPublicIPID string `json:"reservedPublicIpId"`
	CreateNew          bool   `json:"createNew"`
	DisplayName        string `json:"displayName"`
}

type AttachIPv6Request struct {
	VNICID           string `json:"vnicId"`
	IPv6SubnetCIDR   string `json:"ipv6SubnetCidr,omitempty"`
	CIDRPrefixLength int    `json:"cidrPrefixLength,omitempty"`
	AssignMode       string `json:"assignMode,omitempty"` // "auto" | "manual"
	IPAddress        string `json:"ipAddress,omitempty"`
}

type EnableIPv6Request struct {
	VNICID                string `json:"vnicId"`
	HexByte               string `json:"hexByte,omitempty"`
	AddDefaultRoute       bool   `json:"addDefaultRoute,omitempty"`
	AddSecurityListEgress bool   `json:"addSecurityListEgress,omitempty"`
	ConfirmIrreversible   bool   `json:"confirmIrreversible,omitempty"`
}

type ChangeIPv6Result struct {
	Success      bool      `json:"success"`
	Message      string    `json:"message"`
	OldIPAddress string    `json:"oldIpAddress,omitempty"`
	NewIPAddress string    `json:"newIpAddress,omitempty"`
	IPv6         *IPv6Info `json:"ipv6,omitempty"`
	Logs         []string  `json:"logs,omitempty"`
}

type IPManager interface {
	GetIPManagement(instanceID string) (*IPManagementInfo, error)
	ChangePrivateIPPublicIP(instanceID, privateIPID string) (*ChangeIPResult, error)
	AttachIPv4(instanceID string, req AttachIPv4Request) ([]string, string, error)
	DeleteIPv4Managed(instanceID string, req DeleteIPv4Request) ([]string, error)
	AttachReservedIPv4Managed(instanceID string, req AttachReservedIPv4Request) (string, []string, error)
	AttachIPv6(instanceID string, req AttachIPv6Request) (*IPv6Info, error)
	ChangeIPv6(instanceID, vnicID, ipv6ID string) (*ChangeIPv6Result, error)
	DeleteIPv6(instanceID, ipv6ID string) (string, error)
	EnableSubnetIPv6(instanceID string, req EnableIPv6Request) (string, []string, error)
}
