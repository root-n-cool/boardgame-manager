package storage_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"boardgames-manager/internal/storage"
)

func TestSave_ValidPDFIsStoredAndReturnsContentAddressedName(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	content := "%PDF-1.4 fake pdf content for testing"
	name, err := store.Save(storage.ManualCategory, strings.NewReader(content), "regolamento.pdf")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.HasSuffix(name, ".pdf") {
		t.Fatalf("expected .pdf extension, got %q", name)
	}

	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != content {
		t.Fatalf("saved content mismatch")
	}
}

func TestSave_RejectsWrongType(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	// Un'immagine PNG con estensione .pdf: il tipo sniffato non corrisponde
	// a quello che ManualCategory si aspetta per ".pdf", quindi va
	// rifiutato anche se l'estensione dichiarata è quella giusta.
	png := []byte("\x89PNG\r\n\x1a\ncertamente non è un pdf, sono byte a caso di test")
	_, err := store.Save(storage.ManualCategory, strings.NewReader(string(png)), "cover.pdf")
	if err != storage.ErrUnsupportedType {
		t.Fatalf("expected ErrUnsupportedType, got %v", err)
	}
}

// TestSave_RejectsAnExecutableRenamedAsPDF è il caso di sicurezza citato
// nel piano: un file che non è affatto un PDF, ma porta l'estensione
// giusta, deve essere rifiutato perché il contenuto sniffato non
// corrisponde. Un ELF (o un mach-O, uno shell script) non sniffa mai
// "application/pdf".
func TestSave_RejectsAnExecutableRenamedAsPDF(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	elfMagic := "\x7fELF" + strings.Repeat("\x00", 60) + "questo è un eseguibile, non un manuale"
	_, err := store.Save(storage.ManualCategory, strings.NewReader(elfMagic), "manuale.pdf")
	if err != storage.ErrUnsupportedType {
		t.Fatalf("expected ErrUnsupportedType for a renamed executable, got %v", err)
	}
}

func TestSave_RejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	tiny := storage.Category{
		Name:     "tiny",
		Types:    map[string]string{".pdf": "application/pdf"},
		MaxBytes: 10,
	}
	_, err := store.Save(tiny, strings.NewReader("%PDF-1.4 this is definitely more than ten bytes"), "regolamento.pdf")
	if err != storage.ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestSave_DeduplicatesIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	content := "%PDF-1.4 identical content"
	name1, err := store.Save(storage.ManualCategory, strings.NewReader(content), "a.pdf")
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	name2, err := store.Save(storage.ManualCategory, strings.NewReader(content), "b.pdf")
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if name1 != name2 {
		t.Fatalf("expected same content-addressed name, got %q and %q", name1, name2)
	}
}

func TestOpen_ReturnsSavedContent(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	content := "%PDF-1.4 open me"
	name, err := store.Save(storage.ManualCategory, strings.NewReader(content), "manuale.pdf")
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	f, err := store.Open(name)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != content {
		t.Fatalf("content mismatch")
	}
}

// --- Step 1 del piano: le quattro estensioni accettate da ManualCategory,
// distinte per nome file perché il solo sniffing non le distingue.

func TestSave_AcceptsAPlainTextFileWithTxtExtension(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	name, err := store.Save(storage.ManualCategory, strings.NewReader("Regole scritte a mano, senza markdown."), "regole.txt")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.HasSuffix(name, ".txt") {
		t.Fatalf("expected .txt extension, got %q", name)
	}
}

func TestSave_AcceptsAMarkdownFileWithMdExtension(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	// http.DetectContentType non distingue affatto un .md da un .txt:
	// entrambi sniffano "text/plain". È l'estensione dichiarata a decidere,
	// non il contenuto: la stessa identica stringa deve produrre un nome
	// ".md" qui e ".txt" nel test sopra.
	name, err := store.Save(storage.ManualCategory, strings.NewReader("# Titolo\n\nRegole in markdown."), "regole.md")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.HasSuffix(name, ".md") {
		t.Fatalf("expected .md extension, got %q", name)
	}
}

// TestSave_AcceptsADocxFile è il caso esplicitamente richiesto dal piano:
// un .docx (uno zip OOXML) sniffa "application/zip", mai un tipo "word",
// quindi ManualCategory deve accettare quel tipo sniffato per l'estensione
// ".docx" o ogni upload di un manuale Word verrebbe respinto.
func TestSave_AcceptsADocxFile(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	// Un .docx vero è uno zip: bastano i quattro byte di firma "PK\x03\x04"
	// perché http.DetectContentType lo riconosca come application/zip,
	// senza dover costruire un intero archivio OOXML per questo test dello
	// store (che verifica solo il routing per estensione, non il parsing
	// del docx: quello vive in internal/manuals).
	zipMagic := "PK\x03\x04" + strings.Repeat("\x00", 40)
	name, err := store.Save(storage.ManualCategory, strings.NewReader(zipMagic), "Regolamento.docx")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.HasSuffix(name, ".docx") {
		t.Fatalf("expected .docx extension, got %q", name)
	}
}

// TestSave_FallsBackToSniffedTypeWithoutAFilename è il caso del download
// della copertina da BGG (games_handlers.go): nessun nome file, solo un
// io.Reader dal corpo della risposta HTTP. CoverCategory non è ambigua
// (jpeg/png/webp sniffano tre tipi distinti), quindi il ripiego funziona.
func TestSave_FallsBackToSniffedTypeWithoutAFilename(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 40)
	name, err := store.Save(storage.CoverCategory, strings.NewReader(png), "")
	if err != nil {
		t.Fatalf("save without filename: %v", err)
	}
	if !strings.HasSuffix(name, ".png") {
		t.Fatalf("expected .png extension from sniffing alone, got %q", name)
	}
}

// TestSave_WithoutAFilenameRejectsAnAmbiguousSniffedType copre il ramo
// opposto del ripiego sopra: senza nome file, se il tipo sniffato è
// condiviso da più estensioni della categoria (come "text/plain" per .txt
// e .md in ManualCategory), Save deve rifiutare invece di indovinare
// un'estensione a caso.
func TestSave_WithoutAFilenameRejectsAnAmbiguousSniffedType(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	_, err := store.Save(storage.ManualCategory, strings.NewReader("solo testo semplice, nessun nome file"), "")
	if err != storage.ErrUnsupportedType {
		t.Fatalf("expected ErrUnsupportedType for an ambiguous sniff without a filename, got %v", err)
	}
}

// TestSave_StoresABGGCoverDownloadedWithoutAFilename riproduce il percorso
// del download della copertina da BGG (games_handlers.go: Save con
// filename ""), l'unico chiamante di produzione che si affida al solo
// tipo sniffato. Una copertina JPEG deve essere salvata, non rifiutata
// perché CoverCategory elenca due estensioni per lo stesso tipo.
func TestSave_StoresABGGCoverDownloadedWithoutAFilename(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	jpeg := "\xFF\xD8\xFF\xE0\x00\x10JFIF copertina scaricata da BGG"
	name, err := store.Save(storage.CoverCategory, strings.NewReader(jpeg), "")
	if err != nil {
		t.Fatalf("save cover senza filename: %v", err)
	}
	if !strings.HasSuffix(name, ".jpg") {
		t.Fatalf("attesa estensione .jpg, ottenuto %q", name)
	}
}

// TestSave_NormalisesAJpegExtensionToJpg fissa l'altra metà del fix: un
// upload chiamato .jpeg resta accettato, e finisce su disco come .jpg —
// una sola estensione per formato, che è ciò che rende non ambiguo il
// ripiego senza filename.
func TestSave_NormalisesAJpegExtensionToJpg(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	jpeg := "\xFF\xD8\xFF\xE0\x00\x10JFIF copertina caricata a mano"
	name, err := store.Save(storage.CoverCategory, strings.NewReader(jpeg), "copertina.jpeg")
	if err != nil {
		t.Fatalf("save copertina.jpeg: %v", err)
	}
	if !strings.HasSuffix(name, ".jpg") {
		t.Fatalf("attesa estensione .jpg, ottenuto %q", name)
	}
}

// TestSave_AcceptsAPhotoOfARulebookPage: l'admin fotografa una pagina di
// regolamento col telefono e la carica come manuale. JPEG e PNG entrano
// in ManualCategory accanto ai quattro formati di documento.
func TestSave_AcceptsAPhotoOfARulebookPage(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	for _, tc := range []struct{ filename, content, wantExt string }{
		{"pagina.jpg", "\xFF\xD8\xFF\xE0\x00\x10JFIF foto della pagina 3", ".jpg"},
		{"pagina.jpeg", "\xFF\xD8\xFF\xE0\x00\x10JFIF foto della pagina 4", ".jpg"},
		{"schermata.png", "\x89PNG\r\n\x1a\n schermata del regolamento", ".png"},
	} {
		name, err := store.Save(storage.ManualCategory, strings.NewReader(tc.content), tc.filename)
		if err != nil {
			t.Fatalf("save %s: %v", tc.filename, err)
		}
		if !strings.HasSuffix(name, tc.wantExt) {
			t.Fatalf("%s: attesa estensione %s, ottenuto %q", tc.filename, tc.wantExt, name)
		}
	}
}

// TestSave_RejectsAnExecutableRenamedAsAPhoto: le foto non allentano il
// controllo sul contenuto, allo stesso modo del PDF.
func TestSave_RejectsAnExecutableRenamedAsAPhoto(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	elfMagic := "\x7FELF\x02\x01\x01 non sono una foto"
	if _, err := store.Save(storage.ManualCategory, strings.NewReader(elfMagic), "pagina.jpg"); err != storage.ErrUnsupportedType {
		t.Fatalf("atteso ErrUnsupportedType, ottenuto %v", err)
	}
}
