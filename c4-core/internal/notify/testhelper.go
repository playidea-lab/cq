package notify

// OverrideBaseURL replaces the Telegram API base URL used by SendTelegram.
// Intended for tests only — call ResetBaseURL in a defer to restore the default.
func OverrideBaseURL(url string) {
	telegramBaseURL = url
}

// ResetBaseURL restores the default Telegram API base URL.
func ResetBaseURL() {
	telegramBaseURL = "https://api.telegram.org"
}
