// Package geocode cerca luoghi su Nominatim, il servizio di ricerca
// indirizzi di OpenStreetMap. Non serve nessuna chiave: la usage policy
// chiede uno User-Agent che identifichi l'applicazione e un traffico
// leggero, ed è il rate limiter davanti all'handler a garantire il secondo.
package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://nominatim.openstreetmap.org"

// userAgent identifica l'applicazione verso Nominatim, come richiesto dalla
// sua usage policy.
const userAgent = "boardgames-manager (selfhosted board game night manager)"

// maxResults è quante proposte tornano all'admin: una lista che si legge
// tutta senza scorrere, su un telefono come su un portatile.
const maxResults = 8

// Place è un luogo come lo restituisce la ricerca.
type Place struct {
	// Name è l'etichetta breve del posto ("Circolo Arci"); Nominatim la
	// lascia vuota per i risultati che sono solo una via civica.
	Name string
	// Address è il display_name completo, quello che finisce sulla pagina
	// pubblica quando la mappa non c'è.
	Address string
	Lat     float64
	Lon     float64
}

type Client interface {
	Search(ctx context.Context, query string) ([]Place, error)
}

type HTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type placeJSON struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	// Nominatim manda le coordinate come stringhe, anche in jsonv2.
	Lat     string      `json:"lat"`
	Lon     string      `json:"lon"`
	Address addressJSON `json:"address"`
}

// addressJSON sono i pezzi dell'indirizzo, che arrivano solo con
// addressdetails=1. La città cambia nome di campo a seconda di quanto è
// grande il posto: Nominatim la mette in city, town o village.
type addressJSON struct {
	HouseNumber string `json:"house_number"`
	Road        string `json:"road"`
	Postcode    string `json:"postcode"`
	City        string `json:"city"`
	Town        string `json:"town"`
	Village     string `json:"village"`
}

// address compone la riga che si legge su una locandina — "Via Roma 1,
// 10123 Torino" — dal display_name, che infila anche circoscrizione,
// provincia e nazione. Se i pezzi non bastano a fare via e città, il
// display_name intero è comunque meglio di mezza riga.
func (p placeJSON) address() string {
	city := firstNonEmpty(p.Address.City, p.Address.Town, p.Address.Village)
	if p.Address.Road == "" || city == "" {
		return p.DisplayName
	}
	street := p.Address.Road
	if p.Address.HouseNumber != "" {
		street += " " + p.Address.HouseNumber
	}
	if p.Address.Postcode != "" {
		city = p.Address.Postcode + " " + city
	}
	return street + ", " + city
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (c *HTTPClient) Search(ctx context.Context, query string) ([]Place, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, nil
	}

	q := url.Values{}
	q.Set("q", trimmed)
	q.Set("format", "jsonv2")
	q.Set("limit", strconv.Itoa(maxResults))
	q.Set("addressdetails", "1")
	q.Set("accept-language", "it")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/search?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nominatim request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read nominatim response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nominatim returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed []placeJSON
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse nominatim response: %w", err)
	}

	out := make([]Place, 0, len(parsed))
	for _, p := range parsed {
		lat, errLat := strconv.ParseFloat(p.Lat, 64)
		lon, errLon := strconv.ParseFloat(p.Lon, 64)
		// Un risultato senza coordinate leggibili non sa dove mettere il
		// puntino: si scarta invece di piantare una mappa a (0, 0).
		if errLat != nil || errLon != nil {
			continue
		}
		out = append(out, Place{Name: p.Name, Address: p.address(), Lat: lat, Lon: lon})
	}
	return out, nil
}
