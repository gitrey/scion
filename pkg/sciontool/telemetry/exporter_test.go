/*
Copyright 2025 The Scion Authors.
*/

package telemetry

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGCPDialOptions_EmptyPath(t *testing.T) {
	opts, err := loadGCPDialOptions(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts != nil {
		t.Errorf("expected nil options for empty path, got %v", opts)
	}
}

func TestLoadGCPDialOptions_InvalidPath(t *testing.T) {
	_, err := loadGCPDialOptions(context.Background(), "/nonexistent/path/sa.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestLoadGCPDialOptions_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(credFile, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := loadGCPDialOptions(context.Background(), credFile)
	if err == nil {
		t.Fatal("expected error for invalid JSON credentials")
	}
}

func TestLoadGCPDialOptions_ValidKey(t *testing.T) {
	// Generate a test RSA private key
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(privKey)
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privBytes,
	})

	// Build a minimal service account JSON key
	saKey := map[string]string{
		"type":                        "service_account",
		"project_id":                  "test-project",
		"private_key_id":              "key-id",
		"private_key":                 string(privPEM),
		"client_email":                "test@test-project.iam.gserviceaccount.com",
		"client_id":                   "123456789",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        "https://www.googleapis.com/robot/v1/metadata/x509/test",
	}
	keyJSON, err := json.Marshal(saKey)
	if err != nil {
		t.Fatalf("failed to marshal SA key: %v", err)
	}

	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "sa.json")
	if err := os.WriteFile(credFile, keyJSON, 0600); err != nil {
		t.Fatal(err)
	}

	opts, err := loadGCPDialOptions(context.Background(), credFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected non-empty dial options for valid key")
	}
}
