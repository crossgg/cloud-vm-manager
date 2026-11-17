package oci

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"
)

type OCIIAMGroupRef struct {
	MembershipID string `json:"membershipId"`
	GroupID      string `json:"groupId"`
	GroupName    string `json:"groupName"`
}

type OCIIAMUser struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Description    string           `json:"description,omitempty"`
	Email          string           `json:"email,omitempty"`
	LifecycleState string           `json:"lifecycleState"`
	IsMFA          bool             `json:"isMfaActivated"`
	TimeCreated    string           `json:"timeCreated,omitempty"`
	Groups         []OCIIAMGroupRef `json:"groups"`
}

type OCIIAMGroup struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	LifecycleState string `json:"lifecycleState"`
	TimeCreated    string `json:"timeCreated,omitempty"`
}

type OCIIAMCompartment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsRoot      bool   `json:"isRoot,omitempty"`
}

type OCIIAMPolicy struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	CompartmentID  string   `json:"compartmentId"`
	Statements     []string `json:"statements"`
	LifecycleState string   `json:"lifecycleState"`
	TimeCreated    string   `json:"timeCreated,omitempty"`
}

type OCIIAMData struct {
	HomeRegion   string              `json:"homeRegion"`
	TenancyID    string              `json:"tenancyId"`
	Users        []OCIIAMUser        `json:"users"`
	Groups       []OCIIAMGroup       `json:"groups"`
	Compartments []OCIIAMCompartment `json:"compartments"`
	Policies     []OCIIAMPolicy      `json:"policies"`
	PolicyError  string              `json:"policyError,omitempty"`
}

type OCIAPIKeyInfo struct {
	KeyID          string `json:"keyId"`
	Fingerprint    string `json:"fingerprint"`
	KeyValue       string `json:"keyValue,omitempty"`
	LifecycleState string `json:"lifecycleState"`
	TimeCreated    string `json:"timeCreated,omitempty"`
}

type OCICreatedAPIKeyResult struct {
	APIKey        OCIAPIKeyInfo `json:"apiKey"`
	PrivateKeyPEM string        `json:"privateKeyPem,omitempty"`
}

func (o *OCIService) ListIAMData() (*OCIIAMData, error) {
	tenancyID := strings.TrimSpace(o.Account.Tenancy)
	if tenancyID == "" {
		return nil, fmt.Errorf("缺少 Tenancy OCID")
	}
	homeRegion := o.HomeRegion()

	var rawUsers []map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "GET", "/users", url.Values{
		"compartmentId": {tenancyID},
		"limit":         {"1000"},
	}, nil, &rawUsers, false); err != nil {
		return nil, fmt.Errorf("获取用户列表失败: %w", err)
	}

	var rawGroups []map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "GET", "/groups", url.Values{
		"compartmentId": {tenancyID},
		"limit":         {"1000"},
	}, nil, &rawGroups, false); err != nil {
		return nil, fmt.Errorf("获取用户组列表失败: %w", err)
	}

	groups := make([]OCIIAMGroup, 0, len(rawGroups))
	groupNames := make(map[string]string, len(rawGroups))
	userGroups := make(map[string][]OCIIAMGroupRef)

	for _, g := range rawGroups {
		state := strings.ToUpper(stringValue(g["lifecycleState"]))
		if state == "DELETED" || state == "DELETING" {
			continue
		}
		gid := stringValue(g["id"])
		gname := stringValue(g["name"])
		if gid == "" {
			continue
		}
		groupNames[gid] = gname
		groups = append(groups, OCIIAMGroup{
			ID:             gid,
			Name:           gname,
			Description:    stringValue(g["description"]),
			LifecycleState: state,
			TimeCreated:    stringValue(g["timeCreated"]),
		})

		var rawMemberships []map[string]interface{}
		if err := o.identityRequestJSON(homeRegion, "GET", "/userGroupMemberships", url.Values{
			"compartmentId": {tenancyID},
			"groupId":       {gid},
			"limit":         {"1000"},
		}, nil, &rawMemberships, true); err == nil {
			for _, m := range rawMemberships {
				uid := stringValue(m["userId"])
				mid := stringValue(m["id"])
				if uid == "" || mid == "" {
					continue
				}
				userGroups[uid] = append(userGroups[uid], OCIIAMGroupRef{
					MembershipID: mid,
					GroupID:      gid,
					GroupName:    gname,
				})
			}
		}
	}

	users := make([]OCIIAMUser, 0, len(rawUsers))
	for _, u := range rawUsers {
		state := strings.ToUpper(stringValue(u["lifecycleState"]))
		if state == "DELETED" || state == "DELETING" {
			continue
		}
		uid := stringValue(u["id"])
		if uid == "" {
			continue
		}
		isMFA, _ := u["isMfaActivated"].(bool)
		gList := userGroups[uid]
		if gList == nil {
			gList = []OCIIAMGroupRef{}
		}
		users = append(users, OCIIAMUser{
			ID:             uid,
			Name:           stringValue(u["name"]),
			Description:    stringValue(u["description"]),
			Email:          stringValue(u["email"]),
			LifecycleState: state,
			IsMFA:          isMFA,
			TimeCreated:    stringValue(u["timeCreated"]),
			Groups:         gList,
		})
	}

	compartments, _ := o.ListIAMCompartments()
	if len(compartments) == 0 {
		compartments = []OCIIAMCompartment{{
			ID:     tenancyID,
			Name:   "Root (Tenancy)",
			IsRoot: true,
		}}
	}
	policies, policyErr := o.ListIAMPolicies(tenancyID)
	policyErrText := ""
	if policyErr != nil {
		policyErrText = policyErr.Error()
		policies = []OCIIAMPolicy{}
	}

	return &OCIIAMData{
		HomeRegion:   homeRegion,
		TenancyID:    tenancyID,
		Users:        users,
		Groups:       groups,
		Compartments: compartments,
		Policies:     policies,
		PolicyError:  policyErrText,
	}, nil
}

func (o *OCIService) CreateIAMUser(name, description, email, groupID string) (*OCIIAMUser, error) {
	tenancyID := strings.TrimSpace(o.Account.Tenancy)
	if tenancyID == "" {
		return nil, fmt.Errorf("缺少 Tenancy OCID")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("用户名不能为空")
	}
	description = strings.TrimSpace(description)
	if description == "" {
		description = name
	}

	body := map[string]interface{}{
		"compartmentId": tenancyID,
		"name":          name,
		"description":   description,
	}
	if strings.TrimSpace(email) != "" {
		body["email"] = strings.TrimSpace(email)
	}

	homeRegion := o.HomeRegion()
	var created map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "POST", "/users", nil, body, &created, false); err != nil {
		return nil, err
	}
	uid := stringValue(created["id"])
	if strings.TrimSpace(groupID) != "" && uid != "" {
		_ = o.AddUserToGroup(uid, groupID)
	}
	return &OCIIAMUser{
		ID:             uid,
		Name:           stringValue(created["name"]),
		Description:    stringValue(created["description"]),
		Email:          stringValue(created["email"]),
		LifecycleState: stringValue(created["lifecycleState"]),
		Groups:         []OCIIAMGroupRef{},
	}, nil
}

func (o *OCIService) DeleteIAMUser(userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("缺少用户 ID")
	}
	tenancyID := strings.TrimSpace(o.Account.Tenancy)
	homeRegion := o.HomeRegion()

	// Remove memberships first so OCI allows user deletion cleanly
	if tenancyID != "" {
		var memberships []map[string]interface{}
		if err := o.identityRequestJSON(homeRegion, "GET", "/userGroupMemberships", url.Values{
			"compartmentId": {tenancyID},
			"userId":        {userID},
		}, nil, &memberships, true); err == nil {
			for _, m := range memberships {
				if mid := stringValue(m["id"]); mid != "" {
					_ = o.identityRequestJSON(homeRegion, "DELETE", "/userGroupMemberships/"+url.PathEscape(mid), nil, nil, nil, true)
				}
			}
		}
	}

	return o.identityRequestJSON(homeRegion, "DELETE", "/users/"+url.PathEscape(userID), nil, nil, nil, false)
}

func (o *OCIService) ResetIAMUserUIPassword(userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("缺少用户 ID")
	}
	homeRegion := o.HomeRegion()
	var resp map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "POST", "/users/"+url.PathEscape(userID)+"/uiPassword", nil, map[string]interface{}{}, &resp, false); err != nil {
		return "", err
	}
	pw := stringValue(resp["password"])
	if pw == "" {
		return "", fmt.Errorf("OCI 未返回一次性密码")
	}
	return pw, nil
}

func (o *OCIService) ListUserAPIKeys(userID string) ([]OCIAPIKeyInfo, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("缺少用户 ID")
	}
	homeRegion := o.HomeRegion()
	var rawKeys []map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "GET", "/users/"+url.PathEscape(userID)+"/apiKeys", nil, nil, &rawKeys, false); err != nil {
		return nil, err
	}
	keys := make([]OCIAPIKeyInfo, 0, len(rawKeys))
	for _, k := range rawKeys {
		keys = append(keys, OCIAPIKeyInfo{
			KeyID:          stringValue(k["keyId"]),
			Fingerprint:    stringValue(k["fingerprint"]),
			KeyValue:       stringValue(k["keyValue"]),
			LifecycleState: stringValue(k["lifecycleState"]),
			TimeCreated:    stringValue(k["timeCreated"]),
		})
	}
	return keys, nil
}

func (o *OCIService) UploadUserAPIKey(userID, publicKeyPEM string, autoGenerate bool) (*OCICreatedAPIKeyResult, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("缺少用户 ID")
	}

	var privateKeyPEM string
	pubPEM := strings.TrimSpace(publicKeyPEM)
	if autoGenerate {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("生成 RSA 密钥对失败: %w", err)
		}
		privDER, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return nil, err
		}
		privateKeyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}))

		pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
		if err != nil {
			return nil, err
		}
		pubPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	}

	if pubPEM == "" {
		return nil, fmt.Errorf("请提供 PEM 格式公钥或选择自动生成密钥对")
	}

	homeRegion := o.HomeRegion()
	var created map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "POST", "/users/"+url.PathEscape(userID)+"/apiKeys", nil, map[string]interface{}{
		"key": pubPEM,
	}, &created, false); err != nil {
		return nil, err
	}

	return &OCICreatedAPIKeyResult{
		APIKey: OCIAPIKeyInfo{
			KeyID:          stringValue(created["keyId"]),
			Fingerprint:    stringValue(created["fingerprint"]),
			KeyValue:       stringValue(created["keyValue"]),
			LifecycleState: stringValue(created["lifecycleState"]),
			TimeCreated:    stringValue(created["timeCreated"]),
		},
		PrivateKeyPEM: privateKeyPEM,
	}, nil
}

func (o *OCIService) DeleteUserAPIKey(userID, fingerprint string) error {
	userID = strings.TrimSpace(userID)
	fingerprint = strings.TrimSpace(fingerprint)
	if userID == "" || fingerprint == "" {
		return fmt.Errorf("缺少用户 ID 或密钥指纹")
	}
	homeRegion := o.HomeRegion()
	return o.identityRequestJSON(homeRegion, "DELETE", "/users/"+url.PathEscape(userID)+"/apiKeys/"+url.PathEscape(fingerprint), nil, nil, nil, false)
}

func (o *OCIService) CreateIAMGroup(name, description string) (*OCIIAMGroup, error) {
	tenancyID := strings.TrimSpace(o.Account.Tenancy)
	if tenancyID == "" {
		return nil, fmt.Errorf("缺少 Tenancy OCID")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("用户组名称不能为空")
	}
	description = strings.TrimSpace(description)
	if description == "" {
		description = name
	}

	homeRegion := o.HomeRegion()
	var created map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "POST", "/groups", nil, map[string]interface{}{
		"compartmentId": tenancyID,
		"name":          name,
		"description":   description,
	}, &created, false); err != nil {
		return nil, err
	}
	return &OCIIAMGroup{
		ID:             stringValue(created["id"]),
		Name:           stringValue(created["name"]),
		Description:    stringValue(created["description"]),
		LifecycleState: stringValue(created["lifecycleState"]),
	}, nil
}

func (o *OCIService) DeleteIAMGroup(groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("缺少用户组 ID")
	}
	homeRegion := o.HomeRegion()
	return o.identityRequestJSON(homeRegion, "DELETE", "/groups/"+url.PathEscape(groupID), nil, nil, nil, false)
}

func (o *OCIService) AddUserToGroup(userID, groupID string) error {
	tenancyID := strings.TrimSpace(o.Account.Tenancy)
	if tenancyID == "" {
		return fmt.Errorf("缺少 Tenancy OCID")
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(groupID) == "" {
		return fmt.Errorf("缺少用户 ID 或用户组 ID")
	}
	homeRegion := o.HomeRegion()
	return o.identityRequestJSON(homeRegion, "POST", "/userGroupMemberships", nil, map[string]interface{}{
		"compartmentId": tenancyID,
		"userId":        strings.TrimSpace(userID),
		"groupId":       strings.TrimSpace(groupID),
	}, nil, false)
}

func (o *OCIService) RemoveUserFromGroup(membershipID string) error {
	membershipID = strings.TrimSpace(membershipID)
	if membershipID == "" {
		return fmt.Errorf("缺少成员关系 ID")
	}
	homeRegion := o.HomeRegion()
	return o.identityRequestJSON(homeRegion, "DELETE", "/userGroupMemberships/"+url.PathEscape(membershipID), nil, nil, nil, false)
}

func (o *OCIService) ListIAMCompartments() ([]OCIIAMCompartment, error) {
	tenancyID := strings.TrimSpace(o.Account.Tenancy)
	if tenancyID == "" {
		return nil, fmt.Errorf("缺少 Tenancy OCID")
	}
	compartments := []OCIIAMCompartment{{
		ID:     tenancyID,
		Name:   "Root (Tenancy)",
		IsRoot: true,
	}}
	homeRegion := o.HomeRegion()
	var rawCompartments []map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "GET", "/compartments", url.Values{
		"compartmentId":          {tenancyID},
		"compartmentIdInSubtree": {"true"},
		"accessLevel":            {"ANY"},
		"limit":                  {"1000"},
	}, nil, &rawCompartments, true); err != nil {
		return compartments, err
	}
	for _, c := range rawCompartments {
		state := strings.ToUpper(stringValue(c["lifecycleState"]))
		if state == "DELETED" || state == "DELETING" {
			continue
		}
		cid := stringValue(c["id"])
		cname := stringValue(c["name"])
		if cid == "" || cname == "" || cid == tenancyID {
			continue
		}
		compartments = append(compartments, OCIIAMCompartment{
			ID:          cid,
			Name:        cname,
			Description: stringValue(c["description"]),
		})
	}
	return compartments, nil
}

func (o *OCIService) ListIAMPolicies(compartmentID string) ([]OCIIAMPolicy, error) {
	tenancyID := strings.TrimSpace(o.Account.Tenancy)
	if strings.TrimSpace(compartmentID) == "" {
		compartmentID = tenancyID
	}
	if compartmentID == "" {
		return nil, fmt.Errorf("缺少 Tenancy / Compartment OCID")
	}
	homeRegion := o.HomeRegion()
	var rawPolicies []map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "GET", "/policies", url.Values{
		"compartmentId": {compartmentID},
		"limit":         {"1000"},
	}, nil, &rawPolicies, false); err != nil {
		return nil, fmt.Errorf("获取策略列表失败: %w", err)
	}
	policies := make([]OCIIAMPolicy, 0, len(rawPolicies))
	for _, p := range rawPolicies {
		parsed := parseIAMPolicy(p)
		if parsed.ID == "" || parsed.LifecycleState == "DELETED" || parsed.LifecycleState == "DELETING" {
			continue
		}
		policies = append(policies, parsed)
	}
	return policies, nil
}

func (o *OCIService) CreateIAMPolicy(compartmentID, name, description string, statements []string) (*OCIIAMPolicy, error) {
	tenancyID := strings.TrimSpace(o.Account.Tenancy)
	compartmentID = strings.TrimSpace(compartmentID)
	if compartmentID == "" {
		compartmentID = tenancyID
	}
	if compartmentID == "" {
		return nil, fmt.Errorf("缺少 Compartment / Tenancy OCID")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("策略名称不能为空")
	}
	description = strings.TrimSpace(description)
	if description == "" {
		description = name
	}
	cleanStatements := normalizePolicyStatements(statements)
	if len(cleanStatements) == 0 {
		return nil, fmt.Errorf("至少需要一条策略语句")
	}

	homeRegion := o.HomeRegion()
	var created map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "POST", "/policies", nil, map[string]interface{}{
		"compartmentId": compartmentID,
		"name":          name,
		"description":   description,
		"statements":    cleanStatements,
	}, &created, false); err != nil {
		return nil, err
	}
	res := parseIAMPolicy(created)
	return &res, nil
}

func (o *OCIService) UpdateIAMPolicy(policyID, description string, statements []string) (*OCIIAMPolicy, error) {
	policyID = strings.TrimSpace(policyID)
	if policyID == "" {
		return nil, fmt.Errorf("缺少策略 ID")
	}
	cleanStatements := normalizePolicyStatements(statements)
	if len(cleanStatements) == 0 {
		return nil, fmt.Errorf("至少需要一条策略语句")
	}
	body := map[string]interface{}{
		"statements": cleanStatements,
	}
	if desc := strings.TrimSpace(description); desc != "" {
		body["description"] = desc
	}

	homeRegion := o.HomeRegion()
	var updated map[string]interface{}
	if err := o.identityRequestJSON(homeRegion, "PUT", "/policies/"+url.PathEscape(policyID), nil, body, &updated, false); err != nil {
		return nil, err
	}
	res := parseIAMPolicy(updated)
	return &res, nil
}

func (o *OCIService) DeleteIAMPolicy(policyID string) error {
	policyID = strings.TrimSpace(policyID)
	if policyID == "" {
		return fmt.Errorf("缺少策略 ID")
	}
	homeRegion := o.HomeRegion()
	return o.identityRequestJSON(homeRegion, "DELETE", "/policies/"+url.PathEscape(policyID), nil, nil, nil, false)
}

func normalizePolicyStatements(statements []string) []string {
	out := make([]string, 0, len(statements))
	for _, s := range statements {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseIAMPolicy(raw map[string]interface{}) OCIIAMPolicy {
	var statements []string
	if rawStmts, ok := raw["statements"].([]interface{}); ok {
		statements = make([]string, 0, len(rawStmts))
		for _, item := range rawStmts {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				statements = append(statements, s)
			}
		}
	}
	if statements == nil {
		statements = []string{}
	}
	return OCIIAMPolicy{
		ID:             stringValue(raw["id"]),
		Name:           stringValue(raw["name"]),
		Description:    stringValue(raw["description"]),
		CompartmentID:  stringValue(raw["compartmentId"]),
		Statements:     statements,
		LifecycleState: strings.ToUpper(stringValue(raw["lifecycleState"])),
		TimeCreated:    stringValue(raw["timeCreated"]),
	}
}
