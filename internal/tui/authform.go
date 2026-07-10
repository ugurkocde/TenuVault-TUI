package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/config"
)

// startAuthForm builds the app-registration sign-in form for the given method,
// pre-filling tenant/client/cert path from cfg where available.
func (m model) startAuthForm(method config.AuthMethod) (model, tea.Cmd) {
	m.cfg.AuthMethod = method
	tenant := m.cfg.TenantID
	if tenant == "organizations" {
		tenant = ""
	}
	client := m.cfg.ClientID
	if client == config.WellKnownClientID {
		client = ""
	}
	switch method {
	case config.AuthCertificate:
		m.authFormFields = []formField{
			{label: "Tenant ID", value: tenant},
			{label: "Client ID", value: client},
			{label: "Certificate path", value: m.cfg.CertificatePath},
			{label: "Certificate password", masked: true, optional: true},
		}
	default: // client secret
		m.authFormFields = []formField{
			{label: "Tenant ID", value: tenant},
			{label: "Client ID", value: client},
			{label: "Client secret", masked: true},
		}
	}
	m.authFormCursor = 0
	m.goTo(screenAuthForm)
	return m, nil
}

func (m model) keyAuthForm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.goTo(screenAuth)
	case "up", "shift+tab":
		if m.authFormCursor > 0 {
			m.authFormCursor--
		}
	case "down", "tab":
		if m.authFormCursor < len(m.authFormFields)-1 {
			m.authFormCursor++
		}
	case "enter":
		return m.submitAuthForm()
	case "backspace":
		f := &m.authFormFields[m.authFormCursor]
		if n := len(f.value); n > 0 {
			f.value = f.value[:n-1]
		}
	case "space":
		m.authFormFields[m.authFormCursor].value += " "
	default:
		// A single printable rune is appended to the focused field.
		if len([]rune(key)) == 1 {
			m.authFormFields[m.authFormCursor].value += key
		}
	}
	return m, nil
}

func (m model) submitAuthForm() (tea.Model, tea.Cmd) {
	field := func(label string, trim bool) string {
		for _, f := range m.authFormFields {
			if f.label == label {
				if trim {
					return strings.TrimSpace(f.value)
				}
				return f.value
			}
		}
		return ""
	}
	tenant := field("Tenant ID", true)
	client := field("Client ID", true)
	if tenant == "" || client == "" {
		m.status, m.statusKind = "Tenant ID and Client ID are required", "err"
		return m, nil
	}
	m.cfg.TenantID = tenant
	m.cfg.ClientID = client
	if m.cfg.AuthMethod == config.AuthCertificate {
		path := field("Certificate path", true)
		if path == "" {
			m.status, m.statusKind = "Certificate path is required", "err"
			return m, nil
		}
		m.cfg.CertificatePath = path
		m.cfg.CertificatePassword = field("Certificate password", false)
	} else {
		secret := field("Client secret", false)
		if secret == "" {
			m.status, m.statusKind = "Client secret is required", "err"
			return m, nil
		}
		m.cfg.ClientSecret = secret
	}
	return m, m.beginConnect(m.cfg)
}

func (m model) viewAuthForm(w int) string {
	var b strings.Builder
	title := "client secret"
	if m.cfg.AuthMethod == config.AuthCertificate {
		title = "certificate"
	}
	b.WriteString(m.pageHeading("Sign in / App registration", "Authenticate with a "+title+". Credentials are never written to disk.") + "\n\n")
	for i, f := range m.authFormFields {
		lbl := m.th.dim.Render(f.label)
		if f.optional {
			lbl = m.th.dim.Render(f.label) + m.th.cardLabel.Render(" (optional)")
		}
		if i == m.authFormCursor {
			lbl = m.th.selected.Render(f.label)
		}
		var val string
		switch {
		case f.value == "":
			val = m.th.cardLabel.Render("…")
		case f.masked:
			val = m.th.normal.Render(strings.Repeat("•", len([]rune(f.value))))
		default:
			val = m.th.normal.Render(f.value)
		}
		if i == m.authFormCursor {
			val += m.th.accent.Render("█")
		}
		field := lbl + "\n  " + val
		style := m.th.panel.Width(min(w-2, 76))
		if i == m.authFormCursor {
			style = style.BorderForeground(colPurple)
		}
		rendered := style.Render(field)
		start := strings.Count(b.String(), "\n")
		for line := 0; line <= strings.Count(rendered, "\n"); line++ {
			m.recordHitAt(start+line, i)
		}
		b.WriteString(rendered + "\n")
	}
	b.WriteString("\n" + m.th.success.Render("✔ Secrets are kept in memory only"))
	return b.String()
}
