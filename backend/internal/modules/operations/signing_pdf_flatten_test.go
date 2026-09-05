package operations

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/klippa-app/go-pdfium/requests"
	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func signingFlattenTestObjects(objects []string) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.6\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return out.Bytes()
}

func signingFlattenTestStream(dict, content string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%sendstream", dict, len(content), content)
}

func signingFlattenFormFixture(kind string, appearance, needAppearances bool) []byte {
	ap := ""
	field := "/FT /Tx /V (Filled text)"
	appearanceContent := "BT /F1 12 Tf 4 10 Td (Filled text) Tj ET\n"
	if kind == "checkbox" || kind == "unchecked" {
		field = "/FT /Btn /V /Yes /AS /Yes"
		appearanceContent = "2 w 3 3 m 15 15 l 3 15 m 15 3 l S\n"
		if kind == "unchecked" {
			field = "/FT /Btn /V /Off /AS /Off"
		}
	}
	if appearance {
		ap = "/AP << /N 7 0 R >>"
		if kind == "checkbox" {
			ap = "/AP << /N << /Yes 7 0 R /Off 8 0 R >> >>"
		}
	}
	return signingFlattenTestObjects([]string{
		fmt.Sprintf("<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /NeedAppearances %t /DA (/F1 12 Tf 0 g) /DR << /Font << /F1 4 0 R >> >> >> >>", needAppearances),
		"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R /Annots [6 0 R] >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		signingFlattenTestStream("", "BT /F1 16 Tf 50 730 Td (Original contract) Tj ET\n"),
		"<< /Type /Annot /Subtype /Widget /Rect [72 600 300 630] /P 3 0 R /F 4 /T (Field) " + field + " " + ap + " >>",
		signingFlattenTestStream("/Type /XObject /Subtype /Form /BBox [0 0 228 30] /Resources << /Font << /F1 4 0 R >> >>", appearanceContent),
		signingFlattenTestStream("/Type /XObject /Subtype /Form /BBox [0 0 228 30] /Resources << >>", "0 0 228 30 re S\n"),
	})
}

func signingFlattenOutputImage(t *testing.T, data []byte) image.Image {
	t.Helper()
	ctx, err := api.ReadAndValidate(bytes.NewReader(data), signingPDFConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	_, _, attrs, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := ctx.DereferenceDict(attrs.Resources["XObject"])
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range resources {
		stream, _, err := ctx.DereferenceStreamDict(object)
		if err != nil || stream == nil {
			t.Fatalf("image stream missing: %v", err)
		}
		img, err := jpeg.Decode(bytes.NewReader(stream.Raw))
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	t.Fatal("prepared page has no image")
	return nil
}

func signingFlattenInk(img image.Image, x0, y0, x1, y1 float64) int {
	ink := 0
	for y := int(y0 * signingFlattenDPI / 72); y < int(y1*signingFlattenDPI/72); y++ {
		for x := int(x0 * signingFlattenDPI / 72); x < int(x1*signingFlattenDPI/72); x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 35000 && g < 35000 && b < 35000 {
				ink++
			}
		}
	}
	return ink
}

func TestPrepareSigningPDFKeepsSafeDocuments(t *testing.T) {
	input := signingPDFTestDocument("", "", 2)
	output, pages, flattened, err := prepareSigningPDF(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if flattened || !bytes.Equal(input, output) || len(pages) != 2 {
		t.Fatal("safe PDF changed during preparation")
	}
}

func TestPrepareSigningPDFPreservesFormValues(t *testing.T) {
	for _, test := range []struct {
		name, kind                  string
		appearance, needAppearances bool
	}{
		{"text with appearance", "text", true, false},
		{"text needs appearance", "text", false, true},
		{"text missing appearance", "text", false, false},
		{"checked checkbox", "checkbox", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, pages, flattened, err := prepareSigningPDF(context.Background(), signingFlattenFormFixture(test.kind, test.appearance, test.needAppearances))
			if err != nil {
				t.Fatal(err)
			}
			if !flattened || len(pages) != 1 || pages[0] != (SigningPage{Width: 612, Height: 792}) {
				t.Fatalf("unexpected prepared geometry: %#v", pages)
			}
			img := signingFlattenOutputImage(t, output)
			if ink := signingFlattenInk(img, 72, 162, 300, 192); ink < 30 {
				t.Fatalf("form value disappeared (%d dark pixels)", ink)
			}
			if ink := signingFlattenInk(img, 45, 40, 220, 75); ink < 30 {
				t.Fatalf("original text disappeared (%d dark pixels)", ink)
			}
		})
	}
}

func TestPrepareSigningPDFGeneratesCheckboxAppearance(t *testing.T) {
	for _, needsAP := range []bool{false, true} {
		checked, _, _, err := prepareSigningPDF(context.Background(), signingFlattenFormFixture("checkbox", false, needsAP))
		if err != nil {
			t.Fatal(err)
		}
		unchecked, _, _, err := prepareSigningPDF(context.Background(), signingFlattenFormFixture("unchecked", false, needsAP))
		if err != nil {
			t.Fatal(err)
		}
		checkedImage, uncheckedImage := signingFlattenOutputImage(t, checked), signingFlattenOutputImage(t, unchecked)
		checkedInk := signingFlattenInk(checkedImage, 72, 162, 300, 192)
		uncheckedInk := signingFlattenInk(uncheckedImage, 72, 162, 300, 192)
		if checkedInk < uncheckedInk+30 {
			t.Fatalf("missing checkmark after appearance generation: checked=%d, unchecked=%d", checkedInk, uncheckedInk)
		}
	}
}

func TestPrepareSigningPDFNormalizesRotationCropAndActions(t *testing.T) {
	for _, test := range []struct {
		name, page, catalog string
		width, height       float64
	}{
		{"rotation", "/Rotate 90", "", 792, 612},
		{"crop", "/CropBox [20 30 592 762]", "", 572, 732},
		{"actions", "", "/OpenAction << /S /JavaScript /JS (throw Error\\(\\)) >>", 612, 792},
		{"transparent page", "/Group << /S /Transparency /CS /DeviceRGB >>", "/OpenAction << /S /JavaScript /JS (0) >>", 612, 792},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, pages, flattened, err := prepareSigningPDF(context.Background(), signingPDFTestDocument(test.page, test.catalog, 1))
			if err != nil {
				t.Fatal(err)
			}
			if !flattened || len(pages) != 1 || pages[0] != (SigningPage{Width: test.width, Height: test.height}) {
				t.Fatalf("unexpected prepared page: %#v", pages)
			}
			img := signingFlattenOutputImage(t, output)
			r, g, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
			if r < 64000 || g < 64000 || b < 64000 {
				t.Fatal("blank background was not preserved as white")
			}
		})
	}
}

func TestPrepareSigningPDFRejectsUnsafeInput(t *testing.T) {
	for _, input := range [][]byte{
		[]byte("%PDF-broken"), bytes.Repeat([]byte("a"), signingPDFMaxBytes+1),
		signingPDFTestDocument("", "", 51),
		signingPDFTestDocument("", "/AcroForm << /Fields [] /XFA (unsupported) >>", 1),
		signingFlattenTestObjects([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 2000 2000] /Rotate 90 >>"}),
	} {
		if _, _, _, err := prepareSigningPDF(context.Background(), input); err == nil {
			t.Fatal("unsafe input accepted")
		}
	}
	conf := signingPDFConfiguration()
	conf.OwnerPW, conf.EncryptUsingAES, conf.EncryptKeyLength = "owner", true, 256
	var encrypted bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(signingPDFTestDocument("", "", 1)), &encrypted, conf); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := prepareSigningPDF(context.Background(), encrypted.Bytes()); err == nil {
		t.Fatal("encrypted PDF accepted")
	}
}

func TestPrepareSigningPDFPreservesDefaultLayerAndAnnotationVisibility(t *testing.T) {
	input := signingFlattenTestObjects([]string{
		"<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [5 0 R 6 0 R] /D << /BaseState /ON /ON [5 0 R] /OFF [6 0 R] >> >> >>",
		"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Properties << /Visible 5 0 R /Hidden 6 0 R >> >> /Contents 4 0 R /Annots [7 0 R 8 0 R] >>",
		signingFlattenTestStream("", "/OC /Visible BDC 50 700 20 20 re f EMC\n/OC /Hidden BDC 100 700 20 20 re f EMC\n"),
		"<< /Type /OCG /Name (Visible) >>",
		"<< /Type /OCG /Name (Hidden) >>",
		"<< /Type /Annot /Subtype /Square /Rect [150 700 170 720] /F 4 /AP << /N 9 0 R >> >>",
		"<< /Type /Annot /Subtype /Square /Rect [200 700 220 720] /F 32 /AP << /N 9 0 R >> >>",
		signingFlattenTestStream("/Type /XObject /Subtype /Form /BBox [0 0 20 20] /Resources << >>", "0 0 20 20 re f\n"),
	})
	output, _, flattened, err := prepareSigningPDF(context.Background(), input)
	if err != nil || !flattened {
		t.Fatalf("prepare layers and annotations: %v", err)
	}
	img := signingFlattenOutputImage(t, output)
	for _, x := range []float64{50, 150} {
		if ink := signingFlattenInk(img, x+2, 74, x+18, 90); ink < 1000 {
			t.Fatalf("visible content at x=%v disappeared: ink=%d", x, ink)
		}
	}
	for _, x := range []float64{100, 200} {
		if ink := signingFlattenInk(img, x+2, 74, x+18, 90); ink != 0 {
			t.Fatalf("hidden content at x=%v became visible: ink=%d", x, ink)
		}
	}
}

func TestPrepareSigningPDFLargeTransparentPages(t *testing.T) {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Count 8 /Kids [3 0 R 4 0 R 5 0 R 6 0 R 7 0 R 8 0 R 9 0 R 10 0 R] >>",
	}
	for range 8 {
		objects = append(objects, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 1018 1018] /Rotate 90 /Group << /S /Transparency /CS /DeviceRGB >> /Contents 11 0 R >>")
	}
	objects = append(objects, signingFlattenTestStream("", "0 0 20 20 re f\n"))
	output, pages, flattened, err := prepareSigningPDF(context.Background(), signingFlattenTestObjects(objects))
	if err != nil || !flattened || len(pages) != 8 {
		t.Fatalf("large transparent pages: %v, pages=%v", err, pages)
	}
	if len(output) > signingPDFMaxBytes {
		t.Fatal("prepared output exceeded byte limit")
	}
}

func TestPrepareSigningPDFCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := prepareSigningPDF(ctx, signingPDFTestDocument("", "", 1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request returned %v", err)
	}
	signingFlattenSlot <- struct{}{}
	defer func() { <-signingFlattenSlot }()
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, _, _, err := prepareSigningPDF(ctx, signingPDFTestDocument("", "", 1)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued request ignored deadline: %v", err)
	}
}

func TestSigningFlattenBufferBounds(t *testing.T) {
	var output signingFlattenBuffer
	if _, err := output.Write(make([]byte, signingPDFMaxBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write([]byte{1}); err == nil {
		t.Fatal("output exceeded byte limit")
	}
}

func TestSigningFlattenWASMInterruptsExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool, err := newSigningFlattenPool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	engine, err := pool.GetInstanceWithContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	loop := signingFlattenTestObjects([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
		signingFlattenTestStream("", strings.Repeat("0 0 612 792 re f\n", 10000)),
	})
	doc, err := engine.OpenDocument(&requests.OpenDocument{File: &loop})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		result, err := engine.RenderPageInDPI(&requests.RenderPageInDPI{Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: 0}}, DPI: 72})
		if result != nil {
			result.Cleanup()
		}
		done <- err
	}()
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-done:
		t.Fatalf("rendering fixture completed before cancellation: %v", err)
	case <-timer.C:
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("expected interrupted execution, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WASM ignored cancellation")
	}
}
