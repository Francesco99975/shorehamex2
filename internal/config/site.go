package config

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/Francesco99975/shorehamex2/internal/auth"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
)

type ExtraStyle struct {
	CSSFile      string
	CSSIntegrity string
}

type ExtraScript struct {
	JSFile      string
	JSIntegrity string
	Seo         bool
}

type PageMeta struct {
	Title        string
	Description  string
	Indexable    bool
	ExtraStyles  []ExtraStyle
	ExtraScripts []ExtraScript
}

var pageMeta = map[string]PageMeta{
	"/": {
		Title:       "ShorehamEX",
		Description: "Clinical assessment console",
		Indexable:   false,
	},
	"/auth": {
		Title:       "ShorehamEX Auth",
		Description: "Clinical assessment console - Authentication",
		Indexable:   false,
	},
	"/dashboard": {
		Title:       "Dashboard",
		Description: "Clinical assessment console - Dashboard",
		Indexable:   false,
	},
	"/tests": {
		Title:       "Tests",
		Description: "Clinical assessment console - Tests",
		Indexable:   false,
	},
	"/settings": {
		Title:       "Settings",
		Description: "Clinical assessment console - Settings",
		Indexable:   false,
	},
	"/settings/profile": {
		Title:       "Profile",
		Description: "Clinical assessment console - Settings - Profile",
		Indexable:   false,
	},
	"/settings/account": {
		Title:       "Account",
		Description: "Clinical assessment console - Settings - Account",
		Indexable:   false,
	},
	"/settings/security": {
		Title:       "Security",
		Description: "Clinical assessment console - Settings - Security",
		Indexable:   false,
	},
	"/settings/users": {
		Title:       "Users",
		Description: "Clinical assessment console - Settings - Users",
		Indexable:   false,
	},
}

type Organization struct {
	Context      string         `json:"@context"`
	Type         string         `json:"@type"`
	Name         string         `json:"name"`
	Url          string         `json:"url"`
	Logo         string         `json:"logo"`
	ContactPoint []ContactPoint `json:"contactPoint"`
}

type ContactPoint struct {
	Type        string `json:"@type"`
	Telephone   string `json:"telephone"`
	ContactType string `json:"contactType"`
}

type SEO struct {
	Description string
	Keywords    string
	Author      string
	Canonical   string
	Robots      string
}

type URL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	Xmlns   string   `xml:"xmlns,attr"`
	URLs    []URL    `xml:"url"`
}

type Site struct {
	AppName      string
	Title        string
	Metatags     SEO
	Year         int
	CSRF         string
	Nonce        string
	Organization Organization
	Sitemap      []byte
	Styles       []ExtraStyle
	SeoScripts   []ExtraScript
	PageScripts  []ExtraScript
	JSFile       string
	JSIntegrity  string
	CSSFile      string
	CSSIntegrity string
	CurrentUser  *auth.AuthenticatedSessionUser
}

func generateSitemap() []byte {
	baseURL := boot.Environment.URL

	urls := []URL{
		{Loc: baseURL + "/", LastMod: time.Now().Format("2006-01-02")},
		{Loc: baseURL + "/privacy", LastMod: time.Now().Format("2006-01-02")},
		{Loc: baseURL + "/terms", LastMod: time.Now().Format("2006-01-02")},
	}

	sitemap := URLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}

	output, err := xml.MarshalIndent(sitemap, "", "  ")
	if err != nil {
		panic("Sitemap generation failed: " + err.Error())
	}

	return append([]byte(xml.Header), output...)
}
func GetDefaultSite(r *http.Request) Site {

	meta, ok := pageMeta[r.URL.Path]
	if !ok {
		meta = PageMeta{
			Title:       "Error",
			Description: "",
			Indexable:   false,
		}
	}

	jsFile, jsIntegrity := GetJS("index")

	cssFile, cssIntegrity := GetCSS("index")

	protocol := "http"
	if r.TLS != nil {
		protocol = "https"
	}

	canonical := fmt.Sprintf("%s://%s%s", protocol, r.Host, r.URL.Path)

	var robots string
	if meta.Indexable {
		robots = "index, follow"
	} else {
		robots = "noindex, nofollow"
	}

	return Site{
		AppName:  "ShorehamEX",
		Title:    meta.Title,
		Metatags: SEO{Description: meta.Description, Keywords: "", Author: "Francesco Michele Barranca <kalairen>", Canonical: canonical, Robots: robots},
		Year:     time.Now().Year(),
		Organization: Organization{
			Context:      "https://schema.org",
			Type:         "Organization",
			Name:         "ShorehamEX",
			Url:          boot.Environment.URL,
			Logo:         fmt.Sprintf("%s/assets/images/pwa-512x512.png", boot.Environment.URL),
			ContactPoint: []ContactPoint{{Type: "Person", Telephone: "+1-202-555-0144", ContactType: "customer service"}},
		},
		Sitemap:      generateSitemap(),
		Styles:       meta.ExtraStyles,
		SeoScripts:   helpers.FilteredSlice(meta.ExtraScripts, func(es ExtraScript) bool { return es.Seo }),
		PageScripts:  helpers.FilteredSlice(meta.ExtraScripts, func(es ExtraScript) bool { return !es.Seo }),
		JSFile:       jsFile,
		JSIntegrity:  jsIntegrity,
		CSSFile:      cssFile,
		CSSIntegrity: cssIntegrity,
	}
}
