//go:build !darwin

package handlers

func clearMacAppQuarantine(string) error {
	return nil
}
