package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Category descrive un genere di upload: quali estensioni accetta e a
// quale tipo MIME "sniffato" (http.DetectContentType, normalizzato senza
// parametri) ciascuna deve corrispondere, più il tetto di dimensione.
//
// La chiave è l'ESTENSIONE, non il tipo MIME: http.DetectContentType da
// solo non basta a instradare un upload, per due motivi verificati
// leggendo Save prima di questo cambio.
//
//  1. Restituisce parametri (`text/plain; charset=utf-8`), quindi un
//     confronto diretto con una chiave "text/plain" già falliva.
//  2. Non distingue affatto un .md da un .txt (entrambi sniffano
//     "text/plain"), e un .docx (uno zip OOXML) sniffa "application/zip",
//     mai un tipo "word". Il routing per formato che consuma questi file
//     (Task 5 di docs/superpowers/plans) dipende esattamente dalla
//     distinzione che il solo sniffing non può dare.
//
// Types tiene quindi l'estensione dichiarata dal nome del file caricato
// come chiave primaria, e il tipo sniffato atteso come verifica: un file
// rinominato con l'estensione giusta ma un contenuto che non gli
// corrisponde (un eseguibile rinominato .pdf) viene comunque rifiutato,
// perché il tipo sniffato non incrocia quello dichiarato per
// quell'estensione.
type Category struct {
	Name     string
	Types    map[string]string // estensione (con il punto, minuscola) -> tipo MIME sniffato atteso
	MaxBytes int64
}

var ManualCategory = Category{
	Name: "manual",
	Types: map[string]string{
		".pdf":  "application/pdf",
		".txt":  "text/plain",
		".md":   "text/plain",
		".docx": "application/zip",
	},
	MaxBytes: 20 << 20, // 20 MB
}

var CoverCategory = Category{
	Name: "cover",
	Types: map[string]string{
		".jpg":  "image/jpeg",
		".png":  "image/png",
		".webp": "image/webp",
	},
	MaxBytes: 5 << 20, // 5 MB
}

// extAliases riporta alla forma canonica le estensioni che nominano lo
// stesso identico formato. Serve perché Types deve avere UNA sola
// estensione per tipo sniffato: quando Save non riceve un filename (il
// download della copertina da BGG) l'estensione si ricava dal solo tipo
// sniffato, e due chiavi ".jpg"/".jpeg" con lo stesso "image/jpeg"
// rendevano quel ripiego ambiguo — cioè rifiutavano ogni copertina JPEG
// scaricata da BGG.
//
// Non va confuso con l'ambiguità che extensionFor rifiuta apposta: .txt e
// .md sniffano entrambi "text/plain" ma sono formati diversi, che
// l'ingestione tratta in due modi diversi. .jpg e .jpeg sono lo stesso
// formato scritto in due modi, quindi sceglierne uno non indovina niente.
var extAliases = map[string]string{
	".jpeg": ".jpg",
}

func canonicalExt(ext string) string {
	if canonical, ok := extAliases[ext]; ok {
		return canonical
	}
	return ext
}

var ErrUnsupportedType = errors.New("unsupported file type")
var ErrTooLarge = errors.New("file too large")

type Store struct {
	baseDir string
}

func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// sniffContentType è http.DetectContentType normalizzato con
// mime.ParseMediaType: il primo restituisce spesso dei parametri
// ("text/plain; charset=utf-8"), che altrimenti farebbero fallire ogni
// confronto con le chiavi di Category.Types.
func sniffContentType(data []byte) string {
	sniffed := http.DetectContentType(data)
	if mt, _, err := mime.ParseMediaType(sniffed); err == nil {
		return mt
	}
	return sniffed
}

// extensionFor decide quale estensione usare per il nome content-addressed,
// e verifica che il contenuto sia davvero quel che l'estensione promette.
//
// Con un filename: l'estensione dichiarata dal chiamante decide il
// formato (è l'unico modo di distinguere un .md da un .txt, entrambi
// sniffati "text/plain"), ma solo se il tipo sniffato del contenuto
// corrisponde a quanto quell'estensione richiede in category — altrimenti
// un eseguibile rinominato .pdf verrebbe accettato sulla sola fiducia del
// nome.
//
// Senza un filename (il download della copertina da BGG, che ha solo un
// io.Reader dal corpo della risposta HTTP): si ripiega sul tipo sniffato,
// cercando fra le estensioni della categoria l'UNICA il cui tipo atteso
// coincide. Se più di un'estensione condivide lo stesso tipo sniffato
// (come .txt e .md in ManualCategory, formati diversi che l'ingestione
// tratta in due modi diversi) il ripiego è ambiguo e si rifiuta, invece
// di indovinare. Due nomi dello STESSO formato non sono questo caso:
// li appiattisce extAliases prima che arrivino qui, altrimenti ".jpg" e
// ".jpeg" avrebbero reso ambigua CoverCategory — cioè irraggiungibile il
// solo chiamante di produzione senza filename.
func extensionFor(category Category, data []byte, filename string) (string, error) {
	sniffed := sniffContentType(data)

	if filename != "" {
		ext := canonicalExt(strings.ToLower(filepath.Ext(filename)))
		expected, ok := category.Types[ext]
		if !ok || expected != sniffed {
			return "", ErrUnsupportedType
		}
		return ext, nil
	}

	found := ""
	for ext, mt := range category.Types {
		if mt != sniffed {
			continue
		}
		if found != "" && found != ext {
			return "", ErrUnsupportedType // ambiguo: più estensioni, stesso tipo sniffato
		}
		found = ext
	}
	if found == "" {
		return "", ErrUnsupportedType
	}
	return found, nil
}

// Save reads r fully, validates its content type and size against category,
// and writes it to disk under a content-addressed filename (sha256 of the
// content + extension). filename is the originally uploaded file's name
// (used only for its extension, which decides the format when the sniffed
// content type alone cannot — see extensionFor); pass "" when the caller
// has no filename (the BGG cover download in games_handlers.go), and the
// extension falls back to the sniffed content type alone.
//
// Returns the filename (not a full path) to store in the DB. Saving
// identical content twice with the same resulting extension returns the
// same filename without writing a duplicate file.
func (s *Store) Save(category Category, r io.Reader, filename string) (string, error) {
	limited := io.LimitReader(r, category.MaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read upload: %w", err)
	}
	if int64(len(data)) > category.MaxBytes {
		return "", ErrTooLarge
	}

	ext, err := extensionFor(category, data, filename)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	outName := hex.EncodeToString(sum[:]) + ext

	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return "", fmt.Errorf("create uploads dir: %w", err)
	}

	fullPath := filepath.Join(s.baseDir, outName)
	if _, err := os.Stat(fullPath); err == nil {
		return outName, nil
	}

	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write upload: %w", err)
	}

	return outName, nil
}

// Open opens a previously saved file by its filename (as returned by Save).
func (s *Store) Open(filename string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.baseDir, filename))
}
