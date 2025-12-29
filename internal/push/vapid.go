package push

import "os"

// GetPublicKey returns the VAPID public key for client subscription
func GetPublicKey() string {
	return os.Getenv("VAPID_PUBLIC_KEY")
}

// GetPrivateKey returns the VAPID private key for signing
func GetPrivateKey() string {
	return os.Getenv("VAPID_PRIVATE_KEY")
}

// KeysConfigured returns true if VAPID keys are set
func KeysConfigured() bool {
	return GetPublicKey() != "" && GetPrivateKey() != ""
}
