package nswclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ConsignmentAgency is the allowlisted display payload returned by NSW Core
// GET /consignments/{id}/agency. Extra JSON fields from Core are ignored.
type ConsignmentAgency struct {
	ConsignmentID     string `json:"consignmentId"`
	TraderCompanyName string `json:"traderCompanyName"`
}

// GetConsignmentAgency fetches the trader company name from NSW Core.
func (c *Client) GetConsignmentAgency(ctx context.Context, consignmentID string) (*ConsignmentAgency, error) {
	if consignmentID == "" {
		return nil, fmt.Errorf("consignment ID is required")
	}

	apiURL, err := url.JoinPath("consignments", url.PathEscape(consignmentID), "agency")
	if err != nil {
		return nil, fmt.Errorf("failed to build consignment Agency URL: %w", err)
	}

	resp, err := c.http.GetContext(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch consignment Agency data: %w", err)
	}
	defer closeBody(ctx, resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("consignment %s not found", consignmentID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nsw consignment Agency returned status %d", resp.StatusCode)
	}

	var dto ConsignmentAgency
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return nil, fmt.Errorf("failed to decode consignment Agency response: %w", err)
	}
	return &dto, nil
}
