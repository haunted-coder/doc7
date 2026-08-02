//go:build darwin

package credentials

import (
	"errors"
	"os/exec"
	"strings"
)

func keychainPreferred() bool {
	return true
}

func findKeychainPassword(account string) (Result, error) {
	out, err := exec.Command("/usr/bin/security", "find-generic-password", "-a", account, "-s", DefaultService, "-w").CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if strings.Contains(message, "could not be found") || strings.Contains(message, "The specified item could not be found") {
			return Result{}, nil
		}
		return Result{}, errors.New("failed to read doc7 keychain credential")
	}
	key := strings.TrimSpace(string(out))
	if key == "" {
		return Result{}, nil
	}
	return Result{Key: key, Source: "keychain:" + account}, nil
}

func storeKeychainPassword(account string, apiKey string) (string, error) {
	command := exec.Command("/usr/bin/security", "add-generic-password", "-a", account, "-s", DefaultService, "-U", "-w")
	command.Stdin = strings.NewReader(apiKey + "\n")
	if _, err := command.CombinedOutput(); err != nil {
		return "", errors.New("failed to store doc7 keychain credential")
	}
	return "keychain:" + account, nil
}
