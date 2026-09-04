package workspace

import (
	"net/mail"
	"net/url"
	"strings"
)

const maxAccountWebsites = 25

func normalizeAccount(account *Account) error {
	account.Name = strings.TrimSpace(account.Name)
	account.BillingEmail = strings.ToLower(strings.TrimSpace(account.BillingEmail))
	account.Status = strings.ToLower(strings.TrimSpace(account.Status))
	account.Notes = strings.TrimSpace(account.Notes)
	if account.Status == "" {
		account.Status = "prospect"
	}
	websites, err := normalizeWebsites(accountWebsites(*account))
	if err != nil {
		return err
	}
	account.Websites = websites
	if len(websites) > 0 {
		account.Website = websites[0].URL
	} else {
		account.Website = ""
	}
	return nil
}

func validateAccount(account Account) string {
	if account.Name == "" || len(account.Name) > 160 {
		return "Account name must be between 1 and 160 characters"
	}
	if !contains([]string{"prospect", "customer", "inactive"}, account.Status) {
		return "Account status must be prospect, customer, or inactive"
	}
	if account.BillingEmail != "" {
		address, err := mail.ParseAddress(account.BillingEmail)
		if err != nil || !strings.EqualFold(address.Address, account.BillingEmail) {
			return "Enter a valid billing email address"
		}
	}
	if len(account.Websites) > maxAccountWebsites {
		return "An account can have up to 25 websites"
	}
	return ""
}

func normalizeAccountPatch(patch *AccountPatch) error {
	trimPointer(patch.Name, false)
	trimPointer(patch.BillingEmail, true)
	trimPointer(patch.Status, true)
	trimPointer(patch.Notes, false)
	if patch.Websites != nil {
		websites, err := normalizeWebsites(*patch.Websites)
		if err != nil {
			return err
		}
		patch.Websites = &websites
	}
	return nil
}

func validateAccountPatch(patch AccountPatch) string {
	account := Account{Name: "Valid", Status: "prospect"}
	if patch.Name != nil {
		account.Name = *patch.Name
	}
	if patch.BillingEmail != nil {
		account.BillingEmail = *patch.BillingEmail
	}
	if patch.Status != nil {
		account.Status = *patch.Status
	}
	if patch.Websites != nil {
		account.Websites = *patch.Websites
	}
	return validateAccount(account)
}

func accountWebsites(account Account) []Website {
	websites := append([]Website(nil), account.Websites...)
	if len(websites) == 0 && strings.TrimSpace(account.Website) != "" {
		websites = append(websites, Website{URL: account.Website})
	}
	return websites
}

func normalizeWebsites(websites []Website) ([]Website, error) {
	if len(websites) > maxAccountWebsites {
		return nil, &validationError{"An account can have up to 25 websites"}
	}
	result := make([]Website, 0, len(websites))
	seen := make(map[string]struct{})
	for _, website := range websites {
		website.Provider = ""
		website.ExternalID = ""
		website.RenewalDate = ""
		website.AutoRenew = false
		website.Status = ""
		website.URL = strings.TrimSpace(website.URL)
		website.Domain = strings.ToLower(strings.TrimSpace(website.Domain))
		if website.URL == "" && website.Domain != "" {
			website.URL = "https://" + website.Domain
		}
		if website.URL == "" {
			continue
		}
		if !strings.Contains(website.URL, "://") {
			website.URL = "https://" + website.URL
		}
		parsed, err := url.Parse(website.URL)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, &validationError{"Enter a valid website address"}
		}
		parsed.Fragment = ""
		website.URL = parsed.String()
		website.Domain = strings.ToLower(parsed.Hostname())
		key := strings.ToLower(strings.TrimSuffix(website.URL, "/"))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, website)
	}
	return result, nil
}

func mergeWebsite(websites []Website, website Website) []Website {
	for index := range websites {
		if strings.EqualFold(websites[index].Domain, website.Domain) {
			websites[index] = website
			return websites
		}
	}
	return append(websites, website)
}

func preserveManagedWebsiteMetadata(existing, requested []Website) []Website {
	managed := make(map[string]Website, len(existing))
	for _, website := range existing {
		if website.Provider != "" {
			managed[strings.ToLower(website.Domain)] = website
		}
	}
	result := append([]Website(nil), requested...)
	for index, website := range result {
		current, ok := managed[strings.ToLower(website.Domain)]
		if !ok {
			continue
		}
		current.URL = website.URL
		result[index] = current
	}
	return result
}

func applyAccountPatch(account *Account, patch AccountPatch) {
	if patch.Name != nil {
		account.Name = *patch.Name
	}
	if patch.Websites != nil {
		account.Websites = append([]Website(nil), (*patch.Websites)...)
		if len(account.Websites) > 0 {
			account.Website = account.Websites[0].URL
		} else {
			account.Website = ""
		}
	}
	if patch.BillingEmail != nil {
		account.BillingEmail = *patch.BillingEmail
	}
	if patch.Status != nil {
		account.Status = *patch.Status
	}
	if patch.Notes != nil {
		account.Notes = *patch.Notes
	}
}

type validationError struct{ message string }

func (e *validationError) Error() string { return e.message }
