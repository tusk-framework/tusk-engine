package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tusk-framework/tusk-engine/internal/toolchain"
)

const (
	signingKeyEnv   = "TUSK_CATALOG_SIGNING_KEY_B64"
	keyIDEnv        = "TUSK_CATALOG_KEY_ID"
	trustAnchorsEnv = "TUSK_CATALOG_TRUST_ANCHORS_B64"
)

func main() {
	os.Exit(run(os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr))
}

func run(args []string, lookup func(string) (string, bool), stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: tusk-catalog <sign|verify> [flags]")
		return 2
	}
	switch args[0] {
	case "sign":
		return runSign(args[1:], lookup, stdout, stderr)
	case "verify":
		return runVerify(args[1:], lookup, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 2
	}
}

func runSign(args []string, lookup func(string) (string, bool), stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sign", flag.ContinueOnError)
	flags.SetOutput(stderr)
	payloadPath := flags.String("payload", "", "path to the unsigned catalog payload")
	catalogPath := flags.String("catalog", "", "path for the signed catalog output")
	provenancePath := flags.String("provenance", "", "path for the provenance output")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *payloadPath == "" || *catalogPath == "" || *provenancePath == "" {
		fmt.Fprintln(stderr, "sign requires --payload, --catalog, and --provenance")
		return 2
	}

	payloadBytes, err := os.ReadFile(*payloadPath)
	if err != nil {
		return reportError(stderr, "read catalog payload", err)
	}
	payload, err := toolchain.DecodeCatalogPayload(payloadBytes)
	if err != nil {
		return reportError(stderr, "decode catalog payload", err)
	}
	keyID, err := requiredEnvironment(lookup, keyIDEnv)
	if err != nil {
		return reportError(stderr, "read catalog signing configuration", err)
	}
	encodedPrivateKey, err := requiredEnvironment(lookup, signingKeyEnv)
	if err != nil {
		return reportError(stderr, "read catalog signing configuration", err)
	}
	encodedAnchors, err := requiredEnvironment(lookup, trustAnchorsEnv)
	if err != nil {
		return reportError(stderr, "read catalog signing configuration", err)
	}
	privateKeyBytes, err := base64.StdEncoding.DecodeString(encodedPrivateKey)
	if err != nil || len(privateKeyBytes) != ed25519.PrivateKeySize {
		return reportError(stderr, "decode catalog signing key", errors.New("private key must be base64-encoded Ed25519 private key material"))
	}
	anchors, err := toolchain.ParseTrustAnchorsB64(encodedAnchors)
	if err != nil {
		return reportError(stderr, "decode catalog trust anchor", err)
	}
	signedCatalog, err := toolchain.SignCatalog(payload, keyID, ed25519.PrivateKey(privateKeyBytes), anchors)
	if err != nil {
		return reportError(stderr, "sign catalog", err)
	}
	provenance, err := toolchain.GenerateCatalogProvenance(payloadBytes, signedCatalog, keyID)
	if err != nil {
		return reportError(stderr, "generate catalog provenance", err)
	}
	verifier, err := toolchain.CatalogVerifierFromTrustAnchorsB64(encodedAnchors)
	if err != nil {
		return reportError(stderr, "configure catalog verifier", err)
	}
	if _, err := verifier.Verify(signedCatalog); err != nil {
		return reportError(stderr, "verify signed catalog", err)
	}
	if err := writeAtomically(*catalogPath, signedCatalog); err != nil {
		return reportError(stderr, "write signed catalog", err)
	}
	if err := writeAtomically(*provenancePath, provenance); err != nil {
		return reportError(stderr, "write catalog provenance", err)
	}
	fmt.Fprintf(stdout, "signed catalog written to %s\nprovenance written to %s\n", *catalogPath, *provenancePath)
	return 0
}

func runVerify(args []string, lookup func(string) (string, bool), stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	catalogPath := flags.String("catalog", "", "path to the signed catalog")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *catalogPath == "" {
		fmt.Fprintln(stderr, "verify requires --catalog")
		return 2
	}
	encodedAnchors, err := requiredEnvironment(lookup, trustAnchorsEnv)
	if err != nil {
		return reportError(stderr, "read catalog verification configuration", err)
	}
	catalogBytes, err := os.ReadFile(*catalogPath)
	if err != nil {
		return reportError(stderr, "read signed catalog", err)
	}
	verifier, err := toolchain.CatalogVerifierFromTrustAnchorsB64(encodedAnchors)
	if err != nil {
		return reportError(stderr, "configure catalog verifier", err)
	}
	if _, err := verifier.Verify(catalogBytes); err != nil {
		return reportError(stderr, "verify signed catalog", err)
	}
	fmt.Fprintf(stdout, "verified signed catalog %s\n", *catalogPath)
	return 0
}

func requiredEnvironment(lookup func(string) (string, bool), name string) (string, error) {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func reportError(stderr io.Writer, action string, err error) int {
	fmt.Fprintf(stderr, "%s: %v\n", action, err)
	return 1
}

func writeAtomically(path string, data []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}
