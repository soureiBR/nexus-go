package messaging

import (
	"testing"
)

func TestValidateRecipientFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "Group JID",
			input:    "120363123456789012@g.us",
			expected: "120363123456789012@g.us",
			hasError: false,
		},
		{
			name:     "Newsletter JID",
			input:    "123456789@newsletter",
			expected: "123456789@newsletter",
			hasError: false,
		},
		{
			name:     "Broadcaster JID",
			input:    "123456789@broadcaster",
			expected: "123456789@broadcaster",
			hasError: false,
		},
		{
			name:     "Brazilian number with country code",
			input:    "5511988376411",
			expected: "5511988376411@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "Brazilian number with + and country code",
			input:    "+5511988376411",
			expected: "5511988376411@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "Brazilian local number with 9",
			input:    "11988376411",
			expected: "5511988376411@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "Brazilian local number without 9",
			input:    "1188376411",
			expected: "551188376411@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "US number",
			input:    "+15551234567",
			expected: "15551234567@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "Already formatted WhatsApp JID",
			input:    "5511988376411@s.whatsapp.net",
			expected: "5511988376411@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "Empty input",
			input:    "",
			expected: "",
			hasError: true,
		},
		{
			name:     "Invalid characters",
			input:    "abc123",
			expected: "",
			hasError: true,
		},
		{
			name:     "Too short number",
			input:    "123",
			expected: "",
			hasError: true,
		},
		{
			name:     "Brazilian area code 35 - correct format",
			input:    "55358837641",
			expected: "55358837641@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "Brazilian area code 35 with extra 9",
			input:    "5535988376411",
			expected: "5535988376411@s.whatsapp.net",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateRecipientFormat(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error for input %s, but got none", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for input %s: %v", tt.input, err)
				return
			}

			if result != tt.expected {
				t.Errorf("For input %s, expected %s, but got %s", tt.input, tt.expected, result)
			}
		})
	}
}

func TestBrazilianNumberProcessing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Full Brazilian number with 9",
			input:    "5511988376411",
			expected: "5511988376411",
		},
		{
			name:     "Local Brazilian number with 9",
			input:    "11988376411",
			expected: "5511988376411",
		},
		{
			name:     "Local Brazilian number without 9",
			input:    "1188376411",
			expected: "551188376411",
		},
		{
			name:     "International number (non-Brazilian)",
			input:    "15551234567",
			expected: "15551234567",
		},
		{
			name:     "Brazilian number area code 35 (correct format)",
			input:    "55358837641",
			expected: "55358837641",
		},
		{
			name:     "Brazilian number area code 35 with incorrect 9",
			input:    "5535988376411",
			expected: "5535988376411", // Should remain as-is for validation test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processBrazilianNumberFormat(tt.input)
			if result != tt.expected {
				t.Errorf("For input %s, expected %s, but got %s", tt.input, tt.expected, result)
			}
		})
	}
}

func TestRemoveNinthDigitFromBrazilian(t *testing.T) {
	ms := &MessageService{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Brazilian number with 9",
			input:    "5511988376411",
			expected: "551188376411", // Removes the 9 after area code
		},
		{
			name:     "Brazilian number without 9",
			input:    "551188376411",
			expected: "551188376411", // Should remain unchanged
		},
		{
			name:     "Non-Brazilian number",
			input:    "15551234567",
			expected: "15551234567", // Should remain unchanged
		},
		{
			name:     "Wrong length",
			input:    "551198837641",
			expected: "551198837641", // Should remain unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ms.removeNinthDigitFromBrazilian(tt.input)
			if result != tt.expected {
				t.Errorf("For input %s, expected %s, but got %s", tt.input, tt.expected, result)
			}
		})
	}
}

func TestAddNinthDigitToBrazilian(t *testing.T) {
	ms := &MessageService{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Brazilian number without 9 (mobile)",
			input:    "551188376411",
			expected: "5511988376411", // Should add 9
		},
		{
			name:     "Brazilian number already with 9",
			input:    "5511988376411",
			expected: "5511988376411", // Should remain unchanged
		},
		{
			name:     "Brazilian landline (starts with 2 or 3)",
			input:    "551123456789",
			expected: "551123456789", // Should remain unchanged (not mobile)
		},
		{
			name:     "Non-Brazilian number",
			input:    "1555123456",
			expected: "1555123456", // Should remain unchanged
		},
		{
			name:     "Wrong length",
			input:    "5511883764",
			expected: "5511883764", // Should remain unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ms.addNinthDigitToBrazilian(tt.input)
			if result != tt.expected {
				t.Errorf("For input %s, expected %s, but got %s", tt.input, tt.expected, result)
			}
		})
	}
}
