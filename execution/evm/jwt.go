package evm

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/spf13/afero"
)

type JWTSecret [32]byte

func (c *EVMClient) refreshJWTForClient(
	ctx context.Context,
	clientType string,
) {
	c.logger.Info("Starting JWT refresh loop 🔄")
	ticker := time.NewTicker(c.config.RPCJWTRefreshInterval())
	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			if err := c.dialRPCCLient(clientType); err != nil {
				c.logger.Error("Failed to refresh engine auth token: " + err.Error())
			}
			c.logger.Info("Refreshed JWT Token for ethereum engine API")
		}
	}
}

func getJWTFromPath(path string) (JWTSecret, error) {
	var result JWTSecret

	data, err := afero.ReadFile(afero.NewOsFs(), path)
	if err != nil {
		return result, err
	}

	hexString := strings.TrimPrefix(strings.TrimSpace(string(data)), "0x")

	bytes, err := hex.DecodeString(hexString)
	if err != nil {
		return result, fmt.Errorf("failed to decode hex string: %v", err)
	}

	if len(bytes) > 32 {
		return result, fmt.Errorf("hex string too long: got %d bytes, want 32 or fewer", len(bytes))
	}

	copy(result[:], bytes)

	return result, nil
}

func (c *EVMClient) buildJWTHeader() (http.Header, error) {
	header := make(http.Header)

	token, err := buildSignedJWT(c.config.JWTSecret())
	if err != nil {
		return header, err
	}

	header.Set("Authorization", "Bearer "+token)
	return header, nil
}

func buildSignedJWT(s *JWTSecret) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iat": &jwt.NumericDate{Time: time.Now()},
	})
	str, err := token.SignedString(s[:])
	if err != nil {
		return "", errors.Newf("failed to create JWT token: %w", err)
	}
	return str, nil
}
