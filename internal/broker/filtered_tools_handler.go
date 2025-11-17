package broker

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/mark3labs/mcp-go/mcp"
)

// authorizedToolsHeader is a header expected to be set by a trused external source.
var authorizedToolsHeader = http.CanonicalHeaderKey("x-authorized-tools")

const (
	allowedToolsClaimKey = "allowed-tools"
)

// FilterTools will reduce the tool set down to those passed based the authorization lay via the x-authorized-tools header
// The header is expected to be signed as a JWT if we cannot verify the JWT then nothing will be returned
func (broker *mcpBrokerImpl) FilteredTools(_ context.Context, _ any, mcpReq *mcp.ListToolsRequest, mcpRes *mcp.ListToolsResult) {
	originalTools := make([]mcp.Tool, len(mcpRes.Tools))
	copy(originalTools, mcpRes.Tools)
	// set to empty by default
	mcpRes.Tools = []mcp.Tool{}

	var allowedToolsValue string
	if _, ok := mcpReq.Header[authorizedToolsHeader]; !ok {
		broker.logger.Debug("FilteredTools: no tool filtering header sent ", "key", authorizedToolsHeader, "enforced filtering", broker.enforceToolFilter)
		if broker.enforceToolFilter {
			return
		}
		mcpRes.Tools = originalTools
		return
	}
	if len(mcpReq.Header[authorizedToolsHeader]) != 1 {
		broker.logger.Debug("FilteredTools: expected exactly 1 value", "header ", authorizedToolsHeader)
		return
	}

	allowedToolsValue = mcpReq.Header[authorizedToolsHeader][0]

	if allowedToolsValue == "" {
		broker.logger.Debug("FilteredTools: returning no tools", "Header present", authorizedToolsHeader, "empty value specified", allowedToolsValue)
		return
	}
	if broker.trustedHeadersPublicKey == "" {
		broker.logger.Error("no public key provided to validate", "header", authorizedToolsHeader, "header is set and has value", allowedToolsValue)
		return
	}

	// validate the JWT that contains the tools claim
	parsedToken, err := validateJWTHeader(allowedToolsValue, broker.trustedHeadersPublicKey)
	if err != nil {
		broker.logger.Error("did not validate trusted header value ", "header", authorizedToolsHeader, "value", allowedToolsValue, "err", err)
		return
	}
	if claims, ok := parsedToken.Claims.(jwt.MapClaims); ok {
		if toolsValue, ok := claims[allowedToolsClaimKey]; ok {
			allowedToolsValue, ok = toolsValue.(string)
			if !ok {
				broker.logger.Error("failed to retrieve allowed-tools claims from jwt it is not a string")
				return
			}
		} else {
			broker.logger.Error("failed to retrieve allowed-tools claims from jwt")
			return
		}
	} else {
		broker.logger.Error("failed to retrieve claims from jwt")
		return
	}

	broker.logger.Debug("filtering tools based on header", "value", allowedToolsValue)
	authorizedTools := map[string][]string{}
	if err := json.Unmarshal([]byte(allowedToolsValue), &authorizedTools); err != nil {
		broker.logger.Error("failed to unmarshal authorized tools json header returning empty tool set", "error", err)
		return
	}
	mcpRes.Tools = broker.filterTools(authorizedTools)
}

func (broker *mcpBrokerImpl) filterTools(authorizedTools map[string][]string) []mcp.Tool {
	var filteredTools []mcp.Tool
	for server, allowedToolNames := range authorizedTools {
		slog.Debug("checking tools for server ", "server", server, "allowed tools for server", allowedToolNames, "mcpServers", broker.mcpServers)
		// we key off the host so have to iterate for now
		upstreamServer := broker.mcpServers.findByHost(server)
		if upstreamServer == nil {
			broker.logger.Debug("failed to find registered upstream ", "server", server)
			continue
		}
		if upstreamServer.Hostname != server {
			continue
		}

		if upstreamServer.toolsResult == nil {
			broker.logger.Debug("no tools registered for upstream server", "server", upstreamServer.Hostname)
			continue
		}
		broker.logger.Debug("upstream server found ", "upstream ", upstreamServer, "tools", upstreamServer.toolsResult.Tools)
		for _, upstreamTool := range upstreamServer.toolsResult.Tools {
			broker.logger.Debug("checking access ", "tool", upstreamTool.Name, "against", allowedToolNames)
			if slices.Contains(allowedToolNames, upstreamTool.Name) {
				broker.logger.Debug("access granted to", "tool", upstreamTool)
				upstreamTool.Name = string(upstreamServer.prefixedName(upstreamTool.Name))
				filteredTools = append(filteredTools, upstreamTool)
			}
		}
	}
	return filteredTools
}

// validateJWTHeader validates the JWT header expects ES256 alg.
func validateJWTHeader(token string, publicKey string) (*jwt.Token, error) {
	return jwt.Parse(token, func(_ *jwt.Token) (any, error) {
		block, _ := pem.Decode([]byte(publicKey))
		pubkey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		key, ok := pubkey.(*ecdsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("expected a *ecdsa.PublicKey %v", key)
		}
		return key, nil

	}, jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}))
}
