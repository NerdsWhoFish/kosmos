package operations

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/text/encoding/charmap"
)

const signingPDFMaxBytes = 10 << 20
const signingPDFMaxPages = 50

type SigningCertificate struct {
	ID             string
	DocumentTitle  string
	SignerName     string
	SignerEmail    string
	OriginalSHA256 string
	UploadedSHA256 string
	SignedAt       time.Time
	Consent        string
	Session        *SigningSession
}

var signingPDFInit sync.Once

func signingPDFConfiguration() *model.Configuration {
	signingPDFInit.Do(api.DisableConfigDir)
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationStrict
	conf.ValidateLinks = false
	conf.Offline = true
	conf.Limits = model.ResourceLimits{
		MaxStreamBytes:       signingPDFMaxBytes,
		MaxDecodeBytes:       32 << 20,
		MaxImagePixels:       25_000_000,
		MaxImageBytes:        100 << 20,
		MaxObjectCount:       50_000,
		MaxXRefEntries:       50_000,
		MaxObjectStreamCount: 10_000,
		MaxObjectStreamFirst: 1 << 20,
		MaxRecursionDepth:    40,
	}
	return conf
}

func inspectSigningPDF(data []byte) (pages []SigningPage, err error) {
	defer func() {
		if recover() != nil {
			pages, err = nil, errors.New("PDF could not be processed; export a flattened PDF and try again")
		}
	}()
	_, pages, err = readSigningPDF(data)
	return pages, err
}

func readSigningPDF(data []byte) (*model.Context, []SigningPage, error) {
	if len(data) == 0 || len(data) > signingPDFMaxBytes {
		return nil, nil, errors.New("PDF must be between 1 byte and 10 MB")
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, nil, errors.New("upload a valid PDF document")
	}
	ctx, err := api.ReadContext(bytes.NewReader(data), signingPDFConfiguration())
	if err != nil {
		return nil, nil, errors.New("PDF is malformed, encrypted, or exceeds processing limits; export a flattened PDF")
	}
	if ctx.Encrypt != nil {
		return nil, nil, errors.New("encrypted PDFs cannot be signed; export an unencrypted PDF")
	}
	for _, entry := range ctx.Table {
		if entry == nil || entry.Free {
			continue
		}
		if err := checkSigningPDFObject(entry.Object, 0); err != nil {
			return nil, nil, err
		}
	}
	if err := api.ValidateContext(ctx); err != nil {
		return nil, nil, errors.New("PDF failed validation; export a flattened PDF and try again")
	}
	if ctx.PageCount < 1 || ctx.PageCount > signingPDFMaxPages {
		return nil, nil, errors.New("PDF must contain between 1 and 50 pages")
	}
	pages := make([]SigningPage, ctx.PageCount)
	var contentBytes int
	for i := range pages {
		page, _, attrs, err := ctx.PageDict(i+1, false)
		if err != nil || attrs == nil || attrs.MediaBox == nil {
			return nil, nil, errors.New("PDF page geometry could not be read")
		}
		if attrs.Rotate != 0 {
			return nil, nil, fmt.Errorf("page %d has rotation metadata; export a flattened PDF before placing fields", i+1)
		}
		box := attrs.MediaBox
		if box.LL.X != 0 || box.LL.Y != 0 || (attrs.CropBox != nil && *attrs.CropBox != *box) {
			return nil, nil, fmt.Errorf("page %d has unsupported cropping; export a flattened PDF before placing fields", i+1)
		}
		width, height := box.Width(), box.Height()
		if !finiteSigningNumber(width) || !finiteSigningNumber(height) || width < 72 || height < 72 || width > 2880 || height > 2880 {
			return nil, nil, fmt.Errorf("page %d must measure between 1 and 40 inches in each dimension", i+1)
		}
		content, err := ctx.PageContent(page, i+1)
		if err != nil && !errors.Is(err, model.ErrNoContent) {
			return nil, nil, errors.New("PDF page content could not be decoded")
		}
		contentBytes += len(content)
		if contentBytes > 32<<20 {
			return nil, nil, errors.New("PDF page content exceeds processing limits")
		}
		pages[i] = SigningPage{Width: width, Height: height}
	}
	return ctx, pages, nil
}

func checkSigningPDFObject(object types.Object, depth int) error {
	if depth > 40 {
		return errors.New("PDF object nesting exceeds processing limits")
	}
	switch value := object.(type) {
	case types.StreamDict:
		return checkSigningPDFObject(value.Dict, depth+1)
	case types.Dict:
		for key, child := range value {
			switch key {
			case "AcroForm", "XFA", "AA", "OpenAction", "JS", "JavaScript", "EmbeddedFiles", "EF", "RichMedia", "RichMediaContent", "OCProperties", "OC", "Collection", "Alternates", "PresSteps", "A":
				return errors.New("PDF contains forms, actions, attachments, or optional content; export a flattened PDF")
			case "Annots":
				if array, ok := child.(types.Array); !ok || len(array) > 0 {
					return errors.New("PDF contains annotations; export a flattened PDF before signing")
				}
			case "UserUnit":
				if child != types.Integer(1) && child != types.Float(1) {
					return errors.New("PDF uses unsupported page scaling; export a flattened PDF")
				}
			}
			if err := checkSigningPDFObject(child, depth+1); err != nil {
				return err
			}
		}
	case types.Array:
		for _, child := range value {
			if err := checkSigningPDFObject(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func finiteSigningNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validateSigningText(value string) error {
	if !utf8.ValidString(value) {
		return errors.New("text must be valid UTF-8")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return errors.New("text cannot contain control characters or line breaks")
		}
	}
	if _, err := charmap.Windows1252.NewEncoder().String(value); err != nil {
		return errors.New("signing currently supports Western European characters; use a Latin spelling")
	}
	return nil
}

func validateSigningFieldLayout(fields []SigningField, pages []SigningPage) error {
	for _, field := range fields {
		if field.Page < 1 || field.Page > len(pages) || !finiteSigningNumber(field.X) || !finiteSigningNumber(field.Y) || !finiteSigningNumber(field.Width) || !finiteSigningNumber(field.Height) || field.X < 0 || field.Y < 0 || field.Width <= 0 || field.Height <= 0 || field.X+field.Width > 1 || field.Y+field.Height > 1 {
			return errors.New("signing fields must fit within their document page")
		}
		page := pages[field.Page-1]
		if field.Width*page.Width < 20-.0001 || field.Height*page.Height < 15.6-.0001 {
			return errors.New("enlarge each field to at least 20 points wide and 15.6 points high so its text remains readable")
		}
	}
	return nil
}

func validateSigningPreparedFields(fields []SigningField, pages []SigningPage, signerName string) error {
	if err := validateSigningFieldLayout(fields, pages); err != nil {
		return err
	}
	for _, field := range fields {
		value, fontName := "", "Helvetica"
		switch field.Type {
		case "name":
			value = signerName
		case "date":
			value = "2006-01-02"
		case "signature":
			value, fontName = "Signature", "Helvetica-Oblique"
		default:
			continue
		}
		if err := validateSigningText(value); err != nil {
			return err
		}
		encoded, _ := charmap.Windows1252.NewEncoder().String(value)
		page := pages[field.Page-1]
		if _, err := signingFieldFontSize(encoded, fontName, field.Width*page.Width, field.Height*page.Height); err != nil {
			return fmt.Errorf("enlarge the %s field before creating a signing link", field.Type)
		}
	}
	return nil
}

func signingFieldFontSize(encoded, fontName string, width, height float64) (float64, error) {
	textWidth, err := font.TextWidthFloat(encoded, fontName, 1)
	if err != nil || textWidth <= 0 {
		return 0, errors.New("signing text could not be measured")
	}
	size := math.Min(18, math.Min((width-6)/textWidth, (height-6)/1.2))
	if size < 8-.0001 {
		return 0, errors.New("a signing value does not fit its field at a readable size; shorten the value or enlarge the field")
	}
	return math.Max(8, size), nil
}

func renderSigningPDF(data []byte, fields []SigningField, values map[string]string, certificate SigningCertificate) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("signed PDF could not be generated")
		}
	}()
	ctx, pages, err := readSigningPDF(data)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 || len(fields) > 100 {
		return nil, errors.New("document must contain between 1 and 100 signing fields")
	}
	if err := validateSigningFieldLayout(fields, pages); err != nil {
		return nil, err
	}
	streams := make(map[int]*bytes.Buffer)
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if field.ID == "" || seen[field.ID] {
			return nil, errors.New("signing field IDs must be present and unique")
		}
		seen[field.ID] = true
		fontName, fontID := "Helvetica", "KosmosText"
		switch field.Type {
		case "signature":
			fontName, fontID = "Helvetica-Oblique", "KosmosSignature"
		case "name", "text", "date":
		default:
			return nil, errors.New("unsupported signing field type")
		}
		value := strings.TrimSpace(values[field.ID])
		if value == "" {
			if field.Required {
				return nil, errors.New("all required signing fields must be completed")
			}
			continue
		}
		if utf8.RuneCountInString(value) > 200 {
			return nil, errors.New("signing field values cannot exceed 200 characters")
		}
		if err := validateSigningText(value); err != nil {
			return nil, err
		}
		page := pages[field.Page-1]
		x, y := field.X*page.Width, (1-field.Y-field.Height)*page.Height
		width, height := field.Width*page.Width, field.Height*page.Height
		encoded, _ := charmap.Windows1252.NewEncoder().String(value)
		size, err := signingFieldFontSize(encoded, fontName, width, height)
		if err != nil {
			return nil, err
		}
		stream := streams[field.Page]
		if stream == nil {
			stream = new(bytes.Buffer)
			streams[field.Page] = stream
		}
		writeSigningText(stream, encoded, fontID, size, x+3, y+(height-size)/2+size*.2)
	}
	for pageNr, stream := range streams {
		if err := applySigningPDFContent(ctx, pageNr, pages[pageNr-1], stream.Bytes(), true); err != nil {
			return nil, errors.New("signature fields could not be rendered")
		}
	}
	records, err := signingCertificatePages(certificate)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if err := ctx.InsertBlankPages(types.IntSet{ctx.PageCount: true}, &types.Dim{Width: 612, Height: 792}, false); err != nil {
			return nil, errors.New("signing record page could not be added")
		}
		ctx.PageCount++
		if err := applySigningPDFContent(ctx, ctx.PageCount, SigningPage{Width: 612, Height: 792}, record, false); err != nil {
			return nil, errors.New("signing record could not be rendered")
		}
	}
	var output bytes.Buffer
	if err := api.WriteContext(ctx, &output); err != nil {
		return nil, errors.New("signed PDF could not be written")
	}
	// pdfcpu writes PDF 1.7 and its strict validator rejects standard core fonts
	// preserved from older PDFs when those fonts omit optional width tables.
	outputConf := signingPDFConfiguration()
	outputConf.ValidationMode = model.ValidationRelaxed
	if err := api.Validate(bytes.NewReader(output.Bytes()), outputConf); err != nil {
		return nil, errors.New("signed PDF failed output validation")
	}
	return output.Bytes(), nil
}

func writeSigningText(w *bytes.Buffer, encoded, fontID string, size, x, y float64) {
	fmt.Fprintf(w, "q 0 g BT /%s %.4f Tf 1 0 0 1 %.4f %.4f Tm <%s> Tj ET Q\n", fontID, size, x, y, hex.EncodeToString([]byte(encoded)))
}

func applySigningPDFContent(ctx *model.Context, pageNr int, page SigningPage, content []byte, preserve bool) error {
	dict, _, attrs, err := ctx.PageDict(pageNr, false)
	if err != nil {
		return err
	}
	regular, err := signingPDFFont("Helvetica")
	if err != nil {
		return err
	}
	italic, err := signingPDFFont("Helvetica-Oblique")
	if err != nil {
		return err
	}
	resources := types.Dict{"Font": types.Dict{"KosmosText": regular, "KosmosSignature": italic}}
	if preserve {
		original, err := ctx.PageContent(dict, pageNr)
		if err != nil && !errors.Is(err, model.ErrNoContent) {
			return err
		}
		// A Form XObject isolates the contract's graphics state from the signatures.
		form, err := ctx.NewStreamDictForBuf(original)
		if err != nil {
			return err
		}
		form.Dict["Type"] = types.Name("XObject")
		form.Dict["Subtype"] = types.Name("Form")
		form.Dict["BBox"] = types.NewRectangle(0, 0, page.Width, page.Height).Array()
		if attrs.Resources != nil {
			form.Dict["Resources"] = attrs.Resources
		}
		if group := dict["Group"]; group != nil {
			form.Dict["Group"] = group
			delete(dict, "Group")
		}
		if err := form.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*form)
		if err != nil {
			return err
		}
		resources["XObject"] = types.Dict{"KosmosOriginal": *ref}
		content = append([]byte("q /KosmosOriginal Do Q\n"), content...)
	}
	dict["Resources"] = resources
	ref, err := ctx.StreamDictIndRef(content)
	if err != nil {
		return err
	}
	dict["Contents"] = *ref
	return nil
}

func signingPDFFont(name string) (types.Dict, error) {
	widths := make(types.Array, 256)
	for i := range widths {
		width, err := font.CharWidth(name, rune(i))
		if err != nil {
			return nil, err
		}
		widths[i] = types.Integer(width)
	}
	box, err := font.BoundingBox(name)
	if err != nil {
		return nil, err
	}
	flags, angle := 32, 0
	if name == "Helvetica-Oblique" {
		flags, angle = 96, -12
	}
	return types.Dict{
		"Type": types.Name("Font"), "Subtype": types.Name("Type1"), "BaseFont": types.Name(name), "Encoding": types.Name("WinAnsiEncoding"),
		"FirstChar": types.Integer(0), "LastChar": types.Integer(255), "Widths": widths,
		"FontDescriptor": types.Dict{
			"Type": types.Name("FontDescriptor"), "FontName": types.Name(name), "Flags": types.Integer(flags), "FontBBox": box.Array(),
			"ItalicAngle": types.Integer(angle), "Ascent": types.Integer(718), "Descent": types.Integer(-207), "CapHeight": types.Integer(718), "StemV": types.Integer(88),
		},
	}, nil
}

func signingCertificatePages(certificate SigningCertificate) ([][]byte, error) {
	lines := []string{
		"Document: " + certificate.DocumentTitle,
		"Signer name (as supplied): " + certificate.SignerName,
		"Signer email (as supplied): " + certificate.SignerEmail,
		"Signed at (UTC): " + certificate.SignedAt.UTC().Format(time.RFC3339),
		"Document reviewed SHA-256:",
		certificate.OriginalSHA256,
	}
	if certificate.UploadedSHA256 != "" && certificate.UploadedSHA256 != certificate.OriginalSHA256 {
		lines = append(lines,
			"A static copy was prepared from the uploaded PDF before signing.",
			"Uploaded document SHA-256:",
			certificate.UploadedSHA256,
		)
	}
	if session := certificate.Session; session != nil {
		location := make([]string, 0, 3)
		for _, value := range []string{session.City, session.Region, session.Country} {
			if value != "" {
				location = append(location, value)
			}
		}
		approximateLocation, userAgent := strings.Join(location, ", "), session.UserAgent
		if approximateLocation == "" {
			approximateLocation = "Unknown"
		}
		if userAgent == "" {
			userAgent = "Unknown"
		}
		lines = append(lines,
			"Completion session evidence",
			"IP address: "+signingCertificateMetadata(session.IPAddress),
			"Approximate location: "+signingCertificateMetadata(approximateLocation),
			"Session captured at (UTC): "+session.CapturedAt.UTC().Format(time.RFC3339),
			"Evidence source: "+signingCertificateMetadata(session.Source),
			"Browser-reported User-Agent: "+signingCertificateMetadata(userAgent),
			"Location is approximate and may reflect a VPN or proxy.",
			"Connection and browser details are not proof of identity.",
			`Unsupported metadata characters use \uXXXX or \UXXXXXXXX escapes; literal backslashes are doubled.`,
		)
	}
	lines = append(lines,
		"Electronic signing consent:",
		certificate.Consent,
		"Completed through a signing link. Identity and email are self-reported.",
		"This record is an electronic signature audit record, not a PKI digital signature.",
	)
	if len(certificate.ID) > 64 {
		return nil, errors.New("signing request ID is too long")
	}
	if err := validateSigningText(certificate.ID); err != nil {
		return nil, err
	}
	requestID, _ := charmap.Windows1252.NewEncoder().String("Signing request: " + certificate.ID)
	var requestLines []string
	for requestID != "" {
		end, err := signingCertificateLineEnd(requestID)
		if err != nil {
			return nil, err
		}
		requestLines = append(requestLines, requestID[:end])
		requestID = requestID[end:]
	}
	var records [][]byte
	var output bytes.Buffer
	y := 0.0
	startPage := func() {
		output.Reset()
		title := "Signing record"
		if len(records) > 0 {
			title += " (continued)"
		}
		writeSigningText(&output, title, "KosmosText", 20, 48, 744)
		y = 714
		for _, line := range requestLines {
			writeSigningText(&output, line, "KosmosText", 10, 48, y)
			y -= 15
		}
		y -= 15
	}
	finishPage := func() {
		writeSigningText(&output, fmt.Sprintf("Signing record page %d", len(records)+1), "KosmosText", 9, 48, 32)
		records = append(records, bytes.Clone(output.Bytes()))
	}
	startPage()
	for _, line := range lines {
		if len(line) > 8192 {
			return nil, errors.New("signing record text is too long")
		}
		if err := validateSigningText(line); err != nil {
			return nil, err
		}
		encoded, _ := charmap.Windows1252.NewEncoder().String(line)
		for len(encoded) > 0 {
			if y < 48 {
				finishPage()
				if len(records) >= 12 {
					return nil, errors.New("signing record exceeds processing limits")
				}
				startPage()
			}
			end, err := signingCertificateLineEnd(encoded)
			if err != nil {
				return nil, err
			}
			writeSigningText(&output, encoded[:end], "KosmosText", 10, 48, y)
			encoded = encoded[end:]
			y -= 15
		}
		y -= 9
	}
	finishPage()
	return records, nil
}

func signingCertificateMetadata(value string) string {
	var output strings.Builder
	for _, r := range value {
		_, supported := charmap.Windows1252.EncodeRune(r)
		switch {
		case r == '\\':
			output.WriteString(`\\`)
		case !supported || unicode.IsControl(r):
			if r <= 0xffff {
				fmt.Fprintf(&output, `\u%04X`, r)
			} else {
				fmt.Fprintf(&output, `\U%08X`, r)
			}
		default:
			output.WriteRune(r)
		}
	}
	return output.String()
}

func signingCertificateLineEnd(encoded string) (int, error) {
	width := 0.0
	end, lastSpace := 0, 0
	for end < len(encoded) {
		characterWidth, err := font.TextWidthFloat(encoded[end:end+1], "Helvetica", 10)
		if err != nil {
			return 0, errors.New("signing record text could not be measured")
		}
		if width+characterWidth > 516 {
			break
		}
		width += characterWidth
		end++
		if encoded[end-1] == ' ' {
			lastSpace = end
		}
	}
	if end == 0 {
		return 0, errors.New("signing record text could not fit its page")
	}
	if end < len(encoded) && lastSpace > 0 {
		end = lastSpace
	}
	return end, nil
}
