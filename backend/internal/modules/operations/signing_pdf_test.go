package operations

import (
	"bytes"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/text/encoding/charmap"
)

func signingPDFTestDocument(pageExtra, catalogExtra string, pageCount int) []byte {
	// The clip detects signatures inheriting source graphics state.
	content := "BT /F1 16 Tf 50 730 Td (Original contract) Tj ET\n0 0 1 1 re W n\n"
	objects := []string{"", "", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content)}
	var kids strings.Builder
	for i := 0; i < pageCount; i++ {
		fmt.Fprintf(&kids, "%d 0 R ", len(objects)+1)
		objects = append(objects, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents 4 0 R "+pageExtra+" >>")
	}
	objects[0] = "<< /Type /Catalog /Pages 2 0 R " + catalogExtra + " >>"
	objects[1] = fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", pageCount, kids.String())
	var output bytes.Buffer
	output.WriteString("%PDF-1.6\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&output, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}

func TestSigningPDFInspection(t *testing.T) {
	if _, err := api.ReadAndValidate(bytes.NewReader(signingPDFTestDocument("", "", 2)), signingPDFConfiguration()); err != nil {
		t.Fatal(err)
	}
	pages, err := inspectSigningPDF(signingPDFTestDocument("", "", 2))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0] != (SigningPage{Width: 612, Height: 792}) {
		t.Fatalf("unexpected page geometry: %#v", pages)
	}
	for _, test := range []struct {
		name string
		data []byte
	}{
		{"not PDF", []byte("not a PDF")},
		{"malformed", []byte("%PDF-1.7\nbroken")},
		{"oversized", bytes.Repeat([]byte("a"), signingPDFMaxBytes+1)},
		{"too many pages", signingPDFTestDocument("", "", 51)},
		{"rotated", signingPDFTestDocument("/Rotate 90", "", 1)},
		{"cropped", signingPDFTestDocument("/CropBox [10 20 600 770]", "", 1)},
		{"scaled", signingPDFTestDocument("/UserUnit 2", "", 1)},
		{"actions", signingPDFTestDocument("", "/OpenAction << /S /JavaScript /JS (app.alert\\(1\\)) >>", 1)},
		{"forms", signingPDFTestDocument("", "/AcroForm << /Fields [] >>", 1)},
		{"annotations", signingPDFTestDocument("/Annots [<< /Type /Annot /Subtype /Text /Rect [10 10 20 20] /Contents (Change me) >>]", "", 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := inspectSigningPDF(test.data); err == nil {
				t.Fatal("unsupported PDF was accepted")
			}
		})
	}
	t.Run("encrypted even with empty user password", func(t *testing.T) {
		conf := signingPDFConfiguration()
		conf.OwnerPW = "owner password"
		conf.EncryptUsingAES = true
		conf.EncryptKeyLength = 256
		var encrypted bytes.Buffer
		if err := api.Encrypt(bytes.NewReader(signingPDFTestDocument("", "", 1)), &encrypted, conf); err != nil {
			t.Fatal(err)
		}
		if _, err := inspectSigningPDF(encrypted.Bytes()); err == nil {
			t.Fatal("encrypted PDF was accepted")
		}
	})
	t.Run("independent rasterization preserves visible signatures", func(t *testing.T) {
		fields := []SigningField{
			{ID: "signature", Type: "signature", Page: 1, X: .1, Y: .5, Width: .45, Height: .08, Required: true},
			{ID: "date", Type: "date", Page: 1, X: .6, Y: .5, Width: .3, Height: .08, Required: true},
		}
		output, err := renderSigningPDF(signingPDFTestDocument("", "", 1), fields, map[string]string{"signature": "Joey Stout", "date": "2026-09-05"}, signingPDFTestCertificate())
		if err != nil {
			t.Fatal(err)
		}
		tool, err := exec.LookPath("pdftoppm")
		if err != nil {
			t.Skip("optional Poppler raster check requires pdftoppm")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "signed.pdf")
		if err := os.WriteFile(path, output, 0600); err != nil {
			t.Fatal(err)
		}
		prefix := filepath.Join(dir, "signed")
		if result, err := exec.Command(tool, "-f", "1", "-singlefile", "-r", "72", "-png", path, prefix).CombinedOutput(); err != nil {
			t.Fatalf("rasterization failed: %v: %s", err, result)
		}
		file, err := os.Open(prefix + ".png")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		img, err := png.Decode(file)
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != 612 || img.Bounds().Dy() != 792 {
			t.Fatalf("unexpected raster bounds: %v", img.Bounds())
		}
		for _, region := range []struct {
			name           string
			x0, y0, x1, y1 int
		}{
			{"original contract", 45, 40, 200, 70},
			{"signature", 61, 396, 336, 460},
			{"date", 367, 396, 551, 460},
		} {
			ink := 0
			for y := region.y0; y < region.y1; y++ {
				for x := region.x0; x < region.x1; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if r < 50000 && g < 50000 && b < 50000 {
						ink++
					}
				}
			}
			if ink < 30 {
				t.Errorf("%s is not visibly rendered (%d dark pixels)", region.name, ink)
			}
		}
	})
}

func signingPDFTestCertificate() SigningCertificate {
	return SigningCertificate{
		ID: "signing-test-42", DocumentTitle: "Services agreement", SignerName: "Joey Stout", SignerEmail: "joey@example.test",
		OriginalSHA256: strings.Repeat("a", 64), SignedAt: time.Date(2026, 9, 5, 10, 30, 0, 0, time.UTC),
		Consent: "I agree to use electronic records and signatures and intend my signature to be binding.",
	}
}

func TestSigningPDFRendering(t *testing.T) {
	fields := []SigningField{
		{ID: "signature", Type: "signature", Page: 1, X: .1, Y: .5, Width: .45, Height: .08, Required: true},
		{ID: "date", Type: "date", Page: 1, X: .6, Y: .5, Width: .3, Height: .08, Required: true},
		{ID: "text", Type: "text", Page: 2, X: .1, Y: .8, Width: .7, Height: .06, Required: true},
	}
	values := map[string]string{"signature": "Joey Stout", "date": "2026-09-05", "text": `Literal (contract) \\n %p José`}
	output, err := renderSigningPDF(signingPDFTestDocument("", "", 2), fields, values, signingPDFTestCertificate())
	if err != nil {
		t.Fatal(err)
	}
	conf := signingPDFConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	ctx, err := api.ReadAndValidate(bytes.NewReader(output), conf)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.PageCount != 3 {
		t.Fatalf("got %d pages, want contract's two pages plus signing record", ctx.PageCount)
	}
	for pageNr := 1; pageNr <= 2; pageNr++ {
		_, _, attrs, err := ctx.PageDict(pageNr, false)
		if err != nil {
			t.Fatal(err)
		}
		xobjects, err := ctx.DereferenceDict(attrs.Resources["XObject"])
		if err != nil {
			t.Fatal(err)
		}
		form, _, err := ctx.DereferenceStreamDict(xobjects["KosmosOriginal"])
		if err != nil || form == nil {
			t.Fatalf("original contract missing from page %d: %v", pageNr, err)
		}
		if err := form.Decode(); err != nil {
			t.Fatal(err)
		}
		if form.Dict["Subtype"] != types.Name("Form") || !bytes.Contains(form.Content, []byte("Original contract")) {
			t.Fatal("original page content not preserved in isolated form")
		}
	}
	t.Run("independent text extraction and placement", func(t *testing.T) {
		tool, err := exec.LookPath("pdftotext")
		if err != nil {
			t.Skip("optional Poppler PDF extraction check requires pdftotext")
		}
		path := filepath.Join(t.TempDir(), "signed.pdf")
		if err := os.WriteFile(path, output, 0600); err != nil {
			t.Fatal(err)
		}
		extracted, err := exec.Command(tool, "-bbox", path, "-").Output()
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Pages []struct {
				Words []struct {
					Text string  `xml:",chardata"`
					XMin float64 `xml:"xMin,attr"`
					YMin float64 `xml:"yMin,attr"`
					XMax float64 `xml:"xMax,attr"`
					YMax float64 `xml:"yMax,attr"`
				} `xml:"word"`
			} `xml:"body>doc>page"`
		}
		if err := xml.Unmarshal(extracted, &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Pages) != 3 {
			t.Fatalf("independent renderer extracted %d pages", len(doc.Pages))
		}
		foundSignature := false
		for _, word := range doc.Pages[0].Words {
			if word.Text == "Joey" {
				foundSignature = true
				if word.XMin < 61.2 || word.XMax > 336.6 || word.YMin < 396 || word.YMax > 459.36 {
					t.Fatalf("signature outside normalized field bounds: %#v", word)
				}
			}
		}
		if !foundSignature {
			t.Fatal("signature text not independently extractable")
		}
		for page, expected := range map[int][]string{
			0: {"Original", "contract", "Stout", "2026-09-05"},
			1: {"Literal", "(contract)", `\\n`, "%p", "José"},
			2: {"Signing", "record", "signing-test-42", "joey@example.test", strings.Repeat("a", 64), "2026-09-05T10:30:00Z", "self-reported."},
		} {
			actual := make(map[string]bool)
			for _, word := range doc.Pages[page].Words {
				actual[word.Text] = true
			}
			for _, word := range expected {
				if !actual[word] {
					t.Errorf("page %d missing %q in extracted words", page+1, word)
				}
			}
		}
	})
}

func TestSigningPDFRejectsUnreadableOrInvalidFields(t *testing.T) {
	base := SigningField{ID: "sig", Type: "signature", Page: 1, X: .1, Y: .5, Width: .4, Height: .08, Required: true}
	for _, test := range []struct {
		name   string
		mutate func(*SigningField)
		value  string
	}{
		{"unsupported characters", func(*SigningField) {}, "姓名"},
		{"control character", func(*SigningField) {}, "Joey\nStout"},
		{"missing required", func(*SigningField) {}, ""},
		{"too long", func(*SigningField) {}, strings.Repeat("a", 201)},
		{"unreadable", func(f *SigningField) { f.Width = .03 }, "Joey Stout"},
		{"off page", func(f *SigningField) { f.X = .9 }, "Joey Stout"},
		{"non finite", func(f *SigningField) { f.Y = math.NaN() }, "Joey Stout"},
		{"missing page", func(f *SigningField) { f.Page = 2 }, "Joey Stout"},
		{"unknown type", func(f *SigningField) { f.Type = "script" }, "Joey Stout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			field := base
			test.mutate(&field)
			if _, err := renderSigningPDF(signingPDFTestDocument("", "", 1), []SigningField{field}, map[string]string{"sig": test.value}, signingPDFTestCertificate()); err == nil {
				t.Fatal("invalid signing field was accepted")
			}
		})
	}
}

func TestSigningFieldPreparationRejectsUnsignableLinks(t *testing.T) {
	for _, kind := range []string{"height", "date", "name", "signature"} {
		t.Run(kind, func(t *testing.T) {
			_, mux, _ := newTestModule(t)
			item := createSigningFixture(t, mux)
			fields := append([]SigningField(nil), item.Fields...)
			if kind == "height" {
				fields[0].Height = .015
			} else {
				for i := range fields {
					if fields[i].Type == kind {
						fields[i].Width = .05
					}
				}
			}
			path := "/api/v1/signing-requests/" + item.ID
			edit := signingCall(t, mux, "PUT", path, "", map[string]any{"revision": item.Revision, "fields": fields})
			if kind == "height" {
				if edit.Code != 400 {
					t.Fatalf("undersized field edit returned %d: %s", edit.Code, edit.Body.String())
				}
				return
			}
			item = decodeSigningResponse(t, edit, 200)
			link := signingCall(t, mux, "POST", path+"/link", "", map[string]any{"revision": item.Revision, "signerName": "Ada Example", "signerEmail": "ada@example.com", "expiresDays": 7})
			if link.Code != 400 {
				t.Fatalf("unfit %s field link returned %d: %s", kind, link.Code, link.Body.String())
			}
			after := decodeSigningResponse(t, signingCall(t, mux, "GET", path, "", nil), 200)
			if after.Status != "draft" || after.Revision != item.Revision {
				t.Fatal("failed preparation froze or changed the signing request")
			}
		})
	}
}

func TestSigningFieldLayoutUsesPhysicalDimensions(t *testing.T) {
	field := SigningField{ID: "sig", Type: "signature", Label: "Signature", Page: 1, X: .1, Y: .1, Width: .4, Height: .2, Required: true}
	if err := validateSigningFields([]SigningField{field}, []SigningPage{{Width: 72, Height: 72}}, true); err == nil {
		t.Fatal("field smaller than readable height accepted on a small page")
	}
	field.Height = .3
	field.Width = .2
	if err := validateSigningFields([]SigningField{field}, []SigningPage{{Width: 72, Height: 72}}, true); err == nil {
		t.Fatal("field smaller than readable width accepted on a small page")
	}
}

func TestSigningPDFPaginatesUnicodeSessionEvidence(t *testing.T) {
	certificate := signingPDFTestCertificate()
	certificate.UploadedSHA256 = strings.Repeat("b", 64)
	certificate.Session = &SigningSession{
		IPAddress: "2001:db8::1", UserAgent: strings.Repeat("界", 170) + "ab",
		City: strings.Repeat("界", 42) + "ab", Region: strings.Repeat("🌍", 32), Country: "JP",
		CapturedAt: time.Date(2026, 9, 5, 12, 29, 45, 0, time.FixedZone("test", 2*60*60)), Source: "cloudflare",
	}
	fields := []SigningField{{ID: "signature", Type: "signature", Page: 1, X: .1, Y: .5, Width: .45, Height: .08, Required: true}}
	output, err := renderSigningPDF(signingPDFTestDocument("", "", 1), fields, map[string]string{"signature": "Joey Stout"}, certificate)
	if err != nil {
		t.Fatal(err)
	}
	conf := signingPDFConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	ctx, err := api.ReadAndValidate(bytes.NewReader(output), conf)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.PageCount < 3 {
		t.Fatalf("maximum Unicode evidence did not paginate: %d pages", ctx.PageCount)
	}
	var allEvidence strings.Builder
	var lastPageText string
	for pageNr := 2; pageNr <= ctx.PageCount; pageNr++ {
		page, _, _, err := ctx.PageDict(pageNr, false)
		if err != nil {
			t.Fatal(err)
		}
		content, err := ctx.PageContent(page, pageNr)
		if err != nil {
			t.Fatal(err)
		}
		var pageText strings.Builder
		for _, match := range regexp.MustCompile(`<([0-9a-f]+)> Tj`).FindAllSubmatch(content, -1) {
			encoded, err := hex.DecodeString(string(match[1]))
			if err != nil {
				t.Fatal(err)
			}
			text, err := charmap.Windows1252.NewDecoder().String(string(encoded))
			if err != nil {
				t.Fatal(err)
			}
			pageText.WriteString(text)
			if !strings.HasPrefix(text, "Signing record") && !strings.HasPrefix(text, "Signing request:") {
				allEvidence.WriteString(text)
			}
		}
		lastPageText = pageText.String()
		if !strings.Contains(lastPageText, "Signing request: "+certificate.ID) {
			t.Fatalf("record page %d lost its request identifier", pageNr)
		}
		if pageNr > 2 && !strings.Contains(lastPageText, "Signing record (continued)") {
			t.Fatalf("record page %d has no continuation title", pageNr)
		}
	}
	for _, expected := range []string{
		certificate.OriginalSHA256, certificate.UploadedSHA256,
		"IP address: 2001:db8::1", "Session captured at (UTC): 2026-09-05T10:29:45Z", "Evidence source: cloudflare",
		"Approximate location: " + strings.Repeat(`\u754C`, 42) + "ab, " + strings.Repeat(`\U0001F30D`, 32) + ", JP",
		"Browser-reported User-Agent: " + strings.Repeat(`\u754C`, 170) + "ab",
		certificate.Consent, "Connection and browser details are not proof of identity.",
	} {
		if !strings.Contains(allEvidence.String(), expected) {
			t.Fatalf("paginated evidence omitted %q", expected)
		}
	}
	if !strings.Contains(lastPageText, "not a PKI digital signature.") {
		t.Fatal("last record page omitted final signature caveat")
	}
	if certificate.Session.UserAgent != strings.Repeat("界", 170)+"ab" || certificate.Session.Region != strings.Repeat("🌍", 32) {
		t.Fatal("PDF escaped the stored session metadata in place")
	}
	t.Run("independent extraction stays inside page bounds", func(t *testing.T) {
		tool, err := exec.LookPath("pdftotext")
		if err != nil {
			t.Skip("optional independent text bounds check requires pdftotext")
		}
		path := filepath.Join(t.TempDir(), "session-evidence.pdf")
		if err := os.WriteFile(path, output, 0600); err != nil {
			t.Fatal(err)
		}
		extracted, err := exec.Command(tool, "-bbox", path, "-").Output()
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Pages []struct {
				Words []struct {
					Text string  `xml:",chardata"`
					XMin float64 `xml:"xMin,attr"`
					YMin float64 `xml:"yMin,attr"`
					XMax float64 `xml:"xMax,attr"`
					YMax float64 `xml:"yMax,attr"`
				} `xml:"word"`
			} `xml:"body>doc>page"`
		}
		if err := xml.Unmarshal(extracted, &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Pages) != ctx.PageCount {
			t.Fatalf("independent extraction got %d pages, want %d", len(doc.Pages), ctx.PageCount)
		}
		for pageNr, page := range doc.Pages[1:] {
			for _, word := range page.Words {
				if word.XMin < 47.9 || word.XMax > 564.1 || word.YMin < 25 || word.YMax > 765 {
					t.Fatalf("record page %d text outside its margins: %#v", pageNr+1, word)
				}
			}
		}
		var finalText strings.Builder
		for _, word := range doc.Pages[len(doc.Pages)-1].Words {
			finalText.WriteString(word.Text + " ")
		}
		if !strings.Contains(finalText.String(), "not a PKI digital signature.") {
			t.Fatal("final caveat was not independently extractable")
		}
	})
}

func TestSigningCertificateMetadataEscapesReversibly(t *testing.T) {
	if got := signingCertificateMetadata("São \\u754C 界 🌍"); got != `São \\u754C \u754C \U0001F30D` {
		t.Fatalf("ambiguous metadata escaping: %q", got)
	}
	certificate := signingPDFTestCertificate()
	certificate.Session = &SigningSession{IPAddress: "192.0.2.1", Source: "direct", CapturedAt: certificate.SignedAt}
	pages, err := signingCertificatePages(certificate)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Approximate location: Unknown", "Browser-reported User-Agent: Unknown", "Evidence source: direct"} {
		if !bytes.Contains(bytes.Join(pages, nil), []byte(hex.EncodeToString([]byte(expected)))) {
			t.Fatalf("unknown session metadata omitted %q", expected)
		}
	}
}
