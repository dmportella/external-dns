package powerdns

// Covered by original license.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/hashicorp/go-cleanhttp"
)

// Client Powerdns API client.
type Client struct {
	serverURL  string
	apiKey     string
	apiVersion int
	http       *http.Client
}

// NewClient returns a new PowerDNS client
func NewClient(serverURL string, apiKey string) (*Client, error) {
	url, err := url.Parse(serverURL)

	if err != nil {
		return nil, err
	}

	url.Path = ""

	client := Client{
		serverURL: url.String(),
		apiKey:    apiKey,
		http:      cleanhttp.DefaultClient(),
	}
	client.apiVersion, err = client.detectapiVersion()
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// Detects the API version in use on the server
// Uses int to represent the API version: 0 is the legacy AKA version 3.4 API
// Any other integer correlates with the same API version
func (client *Client) detectapiVersion() (int, error) {

	req, err := client.newRequest("GET", "/api/v1/servers", nil)

	if err != nil {
		return -1, err
	}

	resp, err := client.http.Do(req)

	if err != nil {
		return -1, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return 1, nil
	}

	return 0, nil
}

// Creates a new request with necessary headers
func (client *Client) newRequest(method string, endpoint string, body []byte) (*http.Request, error) {
	url, err := url.Parse(client.serverURL)

	if client.apiVersion > 0 {
		url.Path = path.Join("/api/v"+strconv.Itoa(client.apiVersion), endpoint)
	} else {
		url.Path = path.Join(url.Path, endpoint)
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("Error during creation of request: %s", err)
	}

	req.Header.Add("X-API-Key", client.apiKey)
	req.Header.Add("Accept", "application/json")

	if method != "GET" {
		req.Header.Add("Content-Type", "application/json")
	}

	return req, nil
}

// ZoneInfo Data representing Zone Information.
type ZoneInfo struct {
	ID                 string              `json:"ID"`
	Name               string              `json:"name"`
	Account            string              `json:"account"`
	URL                string              `json:"url"`
	LastCheck          int64               `json:"last_check"`
	Kind               string              `json:"kind"`
	DNSSec             bool                `json:"dnsssec"`
	Serial             int64               `json:"serial"`
	NotifiedSerial     int64               `json:"notified_serial"`
	Masters            []string            `json:"masters"`
	Records            []Record            `json:"records,omitempty"`
	ResourceRecordSets []ResourceRecordSet `json:"rrsets,omitempty"`
}

// Record Data representing Record Information.
type Record struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"` // For API v0
	Disabled bool   `json:"disabled"`
}

// ResourceRecordSet Data representing Resource Record Set Information.
type ResourceRecordSet struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	ChangeType string   `json:"changetype"`
	TTL        int      `json:"ttl"` // For API v1
	Records    []Record `json:"records,omitempty"`
}

type zonePatchRequest struct {
	RecordSets []ResourceRecordSet `json:"rrsets"`
}

type errorResponse struct {
	ErrorMsg string `json:"error"`
}

// IDSeparator separator for record identifier.
const IDSeparator string = ":::"

// ID Returns the record identifier.
func (record *Record) ID() string {
	return record.Name + IDSeparator + record.Type
}

// ID Returns the resource record identifier.
func (rrSet *ResourceRecordSet) ID() string {
	return rrSet.Name + IDSeparator + rrSet.Type
}

// Returns name and type of record or record set based on it's ID
func parseID(recID string) (string, string, error) {
	s := strings.Split(recID, IDSeparator)

	if len(s) == 2 {
		return s[0], s[1], nil
	}

	return "", "", fmt.Errorf("Unknown record ID format")
}

// ListZones Returns all Zones of server, without records
func (client *Client) ListZones() ([]ZoneInfo, error) {

	req, err := client.newRequest("GET", "/servers/localhost/zones", nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var zoneInfos []ZoneInfo

	err = json.NewDecoder(resp.Body).Decode(&zoneInfos)
	if err != nil {
		return nil, err
	}

	return zoneInfos, nil
}

// ListRecords Returns all records in Zone
func (client *Client) ListRecords(zone string) ([]Record, error) {
	req, err := client.newRequest("GET", fmt.Sprintf("/servers/localhost/zones/%s", zone), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	zoneInfo := new(ZoneInfo)
	err = json.NewDecoder(resp.Body).Decode(zoneInfo)
	if err != nil {
		return nil, err
	}

	records := zoneInfo.Records
	// Convert the API v1 response to v0 record structure
	for _, rrs := range zoneInfo.ResourceRecordSets {
		for _, record := range rrs.Records {
			records = append(records, Record{
				Name:    rrs.Name,
				Type:    rrs.Type,
				Content: record.Content,
				TTL:     rrs.TTL,
			})
		}
	}

	return records, nil
}

// ListRecordsAsRRSet Returns only records of specified name and type
func (client *Client) ListRecordsAsRRSet(zone string) ([]ResourceRecordSet, error) {
	req, err := client.newRequest("GET", fmt.Sprintf("/servers/localhost/zones/%s", zone), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	zoneInfo := new(ZoneInfo)
	err = json.NewDecoder(resp.Body).Decode(zoneInfo)
	if err != nil {
		return nil, err
	}

	if zoneInfo.ResourceRecordSets == nil && len(zoneInfo.ResourceRecordSets) == 0 {
		return nil, nil
	}

	return zoneInfo.ResourceRecordSets, nil
}

// ListRecordsByNameAndType Returns only records of specified name and type
func (client *Client) ListRecordsByNameAndType(zone string, name string, tpe string) ([]Record, error) {
	allRecords, err := client.ListRecords(zone)
	if err != nil {
		return nil, err
	}

	records := make([]Record, 0, 10)
	for _, r := range allRecords {
		if r.Name == name && r.Type == tpe {
			records = append(records, r)
		}
	}

	return records, nil
}

// ListRecordsByID returns only records that match the specified record IDentifier.
func (client *Client) ListRecordsByID(zone string, recID string) ([]Record, error) {
	name, tpe, err := parseID(recID)

	if err != nil {
		return nil, err
	}

	return client.ListRecordsByNameAndType(zone, name, tpe)
}

// RecordExists Checks if requested record exists in Zone
func (client *Client) RecordExists(zone string, name string, tpe string) (bool, error) {
	allRecords, err := client.ListRecords(zone)
	if err != nil {
		return false, err
	}

	for _, record := range allRecords {
		if record.Name == name && record.Type == tpe {
			return true, nil
		}
	}

	return false, nil
}

// RecordExistsByID Checks if requested record exists in Zone by it's ID
func (client *Client) RecordExistsByID(zone string, recID string) (bool, error) {
	name, tpe, err := parseID(recID)

	if err != nil {
		return false, err
	}

	return client.RecordExists(zone, name, tpe)
}

// CreateRecord Creates new record with single content entry
func (client *Client) CreateRecord(zone string, record Record) (string, error) {
	reqBody, _ := json.Marshal(zonePatchRequest{
		RecordSets: []ResourceRecordSet{
			{
				Name:       record.Name,
				Type:       record.Type,
				ChangeType: "REPLACE",
				Records:    []Record{record},
			},
		},
	})

	req, err := client.newRequest("PATCH", fmt.Sprintf("/servers/localhost/zones/%s", zone), reqBody)
	if err != nil {
		return "", err
	}

	resp, err := client.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		errorResp := new(errorResponse)
		if err = json.NewDecoder(resp.Body).Decode(errorResp); err != nil {
			return "", fmt.Errorf("Error creating record: %s", record.ID())
		}

		return "", fmt.Errorf("Error creating record: %s, reason: %q", record.ID(), errorResp.ErrorMsg)
	}

	return record.ID(), nil
}

// ReplaceRecordSet Creates new record set in Zone
func (client *Client) ReplaceRecordSet(zone string, rrSet ResourceRecordSet) (string, error) {
	rrSet.ChangeType = "REPLACE"

	reqBody, _ := json.Marshal(zonePatchRequest{
		RecordSets: []ResourceRecordSet{rrSet},
	})

	req, err := client.newRequest("PATCH", fmt.Sprintf("/servers/localhost/zones/%s", zone), reqBody)
	if err != nil {
		return "", err
	}

	resp, err := client.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		errorResp := new(errorResponse)
		if err = json.NewDecoder(resp.Body).Decode(errorResp); err != nil {
			return "", fmt.Errorf("Error creating record set: %s", rrSet.ID())
		}

		return "", fmt.Errorf("Error creating record set: %s, reason: %q", rrSet.ID(), errorResp.ErrorMsg)
	}

	return rrSet.ID(), nil
}

// DeleteRecordSet Deletes record set from Zone
func (client *Client) DeleteRecordSet(zone string, name string, tpe string) error {
	reqBody, _ := json.Marshal(zonePatchRequest{
		RecordSets: []ResourceRecordSet{
			{
				Name:       name,
				Type:       tpe,
				ChangeType: "DELETE",
			},
		},
	})

	req, err := client.newRequest("PATCH", fmt.Sprintf("/servers/localhost/zones/%s", zone), reqBody)
	if err != nil {
		return err
	}

	resp, err := client.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		errorResp := new(errorResponse)
		if err = json.NewDecoder(resp.Body).Decode(errorResp); err != nil {
			return fmt.Errorf("Error deleting record: %s %s", name, tpe)
		}

		return fmt.Errorf("Error deleting record: %s %s, reason: %q", name, tpe, errorResp.ErrorMsg)
	}

	return nil
}

// DeleteRecordSetByID Deletes record from Zone by it's ID
func (client *Client) DeleteRecordSetByID(zone string, recID string) error {
	name, tpe, err := parseID(recID)
	if err != nil {
		return err
	}

	return client.DeleteRecordSet(zone, name, tpe)
}
