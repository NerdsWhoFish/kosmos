package operations

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"math"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/tetratelabs/wazero"
)

const signingFlattenDPI = 200
const signingFlattenMaxPixels = 8_000_000
const signingFlattenTimeout = 45 * time.Second

var signingFlattenSlot = make(chan struct{}, 1)
var signingFlattenCompilationCache = wazero.NewCompilationCache()

func prepareSigningPDF(ctx context.Context, data []byte) (prepared []byte, pages []SigningPage, flattened bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, signingFlattenTimeout)
	defer cancel()
	defer func() {
		if recover() != nil {
			prepared, pages, flattened, err = nil, nil, false, errors.New("PDF could not be prepared safely; export a flattened PDF and try again")
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, nil, false, err
	}
	select {
	case signingFlattenSlot <- struct{}{}:
		defer func() { <-signingFlattenSlot }()
	case <-ctx.Done():
		return nil, nil, false, ctx.Err()
	}
	if len(data) == 0 || len(data) > signingPDFMaxBytes || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, nil, false, errors.New("upload a valid PDF no larger than 10 MB")
	}
	if pages, err := inspectSigningPDF(data); err == nil {
		return data, pages, false, nil
	}
	if err := checkSigningFlattenInput(data); err != nil {
		return nil, nil, false, err
	}
	prepared, pages, err = flattenSigningPDF(ctx, data)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, false, ctx.Err()
		}
		return nil, nil, false, err
	}
	return prepared, pages, true, nil
}

func checkSigningFlattenInput(data []byte) error {
	conf := signingPDFConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	parsed, err := api.ReadContext(bytes.NewReader(data), conf)
	if err != nil {
		return errors.New("PDF is malformed, encrypted, or exceeds processing limits")
	}
	if parsed.Encrypt != nil {
		return errors.New("encrypted PDFs cannot be prepared for signing; export an unencrypted PDF")
	}
	catalog, err := parsed.Catalog()
	if err != nil {
		return errors.New("PDF catalog could not be read")
	}
	form, err := parsed.DereferenceDict(catalog["AcroForm"])
	if err != nil {
		return errors.New("PDF form could not be read")
	}
	if form != nil && form["XFA"] != nil {
		return errors.New("XFA forms cannot be prepared safely; export a standard PDF first")
	}
	if err := api.ValidateContext(parsed); err != nil {
		return errors.New("PDF failed validation; export a flattened PDF and try again")
	}
	if parsed.PageCount < 1 || parsed.PageCount > signingPDFMaxPages {
		return errors.New("PDF must contain between 1 and 50 pages")
	}
	return nil
}

func newSigningFlattenPool(ctx context.Context) (pdfium.Pool, error) {
	return webassembly.Init(webassembly.Config{
		Context:  ctx,
		MaxIdle:  1,
		MaxTotal: 1,
		FSConfig: wazero.NewFSConfig(),
		Stdout:   io.Discard,
		Stderr:   io.Discard,
		RuntimeConfig: wazero.NewRuntimeConfig().
			WithCompilationCache(signingFlattenCompilationCache).
			WithMemoryLimitPages(3072).
			WithCloseOnContextDone(true),
	})
}

func flattenSigningPDF(ctx context.Context, data []byte) ([]byte, []SigningPage, error) {
	pool, err := newSigningFlattenPool(ctx)
	if err != nil {
		return nil, nil, errors.New("PDF preparation engine could not start")
	}
	defer pool.Close()
	engine, err := pool.GetInstanceWithContext(ctx)
	if err != nil {
		return nil, nil, errors.New("PDF preparation engine is unavailable")
	}
	defer engine.Close()
	document, err := engine.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, nil, errors.New("PDF could not be opened for preparation")
	}
	security, err := engine.FPDF_GetSecurityHandlerRevision(&requests.FPDF_GetSecurityHandlerRevision{Document: document.Document})
	if err != nil || security.SecurityHandlerRevision != -1 {
		return nil, nil, errors.New("encrypted PDFs cannot be prepared for signing")
	}
	form, err := engine.FPDF_GetFormType(&requests.FPDF_GetFormType{Document: document.Document})
	if err != nil || (form.FormType != enums.FPDF_FORMTYPE_NONE && form.FormType != enums.FPDF_FORMTYPE_ACRO_FORM) {
		return nil, nil, errors.New("this PDF form type cannot be prepared safely; export a standard PDF first")
	}
	count, err := engine.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: document.Document})
	if err != nil || count.PageCount < 1 || count.PageCount > signingPDFMaxPages {
		return nil, nil, errors.New("PDF must contain between 1 and 50 pages")
	}
	output, err := pdfcpu.CreateContextWithXRefTable(signingPDFConfiguration(), types.PaperSize["Letter"])
	if err != nil {
		return nil, nil, errors.New("prepared PDF could not be created")
	}
	pagesRoot, err := output.Pages()
	if err != nil || pagesRoot == nil {
		return nil, nil, errors.New("prepared PDF page tree could not be created")
	}
	pageTree, err := output.DereferenceDict(*pagesRoot)
	if err != nil {
		return nil, nil, errors.New("prepared PDF page tree could not be read")
	}
	imageBytes := 0
	for pageNr := 0; pageNr < count.PageCount; pageNr++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		page := requests.Page{ByIndex: &requests.PageByIndex{Document: document.Document, Index: pageNr}}
		width, err := engine.FPDF_GetPageWidth(&requests.FPDF_GetPageWidth{Page: page})
		if err != nil {
			return nil, nil, errors.New("PDF page dimensions could not be read")
		}
		height, err := engine.FPDF_GetPageHeight(&requests.FPDF_GetPageHeight{Page: page})
		if err != nil {
			return nil, nil, errors.New("PDF page dimensions could not be read")
		}
		if err := validateSigningFlattenDimensions(width.Width, height.Height); err != nil {
			return nil, nil, err
		}
		annotationCount, err := engine.FPDFPage_GetAnnotCount(&requests.FPDFPage_GetAnnotCount{Page: page})
		if err != nil || annotationCount.Count < 0 || annotationCount.Count > 1000 {
			return nil, nil, errors.New("PDF annotations could not be prepared within processing limits")
		}
		if form.FormType == enums.FPDF_FORMTYPE_ACRO_FORM {
			if _, err := engine.GetForm(&requests.GetForm{Page: page}); err != nil {
				return nil, nil, errors.New("PDF form values could not be prepared safely")
			}
		}
		if err := checkSigningWidgetAppearances(engine, page, annotationCount.Count); err != nil {
			return nil, nil, err
		}
		jpegData, err := renderSigningFlattenPage(ctx, engine, page)
		if err != nil {
			return nil, nil, err
		}
		imageBytes += len(jpegData)
		if imageBytes > signingPDFMaxBytes {
			return nil, nil, errors.New("prepared PDF exceeds 10 MB; use a smaller document")
		}
		importConf := pdfcpu.DefaultImportConfig()
		importConf.PageDim = &types.Dim{Width: width.Width, Height: height.Height}
		importConf.Pos, importConf.Scale, importConf.UserDim = types.Center, 1, true
		refs, err := pdfcpu.NewPagesForImage(output.XRefTable, bytes.NewReader(jpegData), pagesRoot, importConf)
		if err != nil || len(refs) != 1 {
			return nil, nil, errors.New("prepared PDF page could not be created")
		}
		if err := output.SetValid(*refs[0]); err != nil {
			return nil, nil, errors.New("prepared PDF page could not be validated")
		}
		if err := model.AppendPageTree(refs[0], 1, pageTree); err != nil {
			return nil, nil, errors.New("prepared PDF page could not be appended")
		}
		output.PageCount++
	}
	var encoded signingFlattenBuffer
	if err := api.WriteContext(output, &encoded); err != nil {
		return nil, nil, errors.New("prepared PDF could not be saved within the 10 MB limit")
	}
	pages, err := inspectSigningPDF(encoded.Bytes())
	if err != nil {
		return nil, nil, errors.New("prepared PDF failed safety validation")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return encoded.Bytes(), pages, nil
}

func validateSigningFlattenDimensions(width, height float64) error {
	if !finiteSigningNumber(width) || !finiteSigningNumber(height) || width < 72 || height < 72 || width > 2880 || height > 2880 {
		return errors.New("PDF pages must measure between 1 and 40 inches in each dimension")
	}
	pixels := math.Ceil(width*signingFlattenDPI/72) * math.Ceil(height*signingFlattenDPI/72)
	if pixels > signingFlattenMaxPixels {
		return errors.New("PDF page is too large to prepare safely at print quality; reduce the page size")
	}
	return nil
}

func checkSigningWidgetAppearances(engine pdfium.Pdfium, page requests.Page, count int) error {
	for i := 0; i < count; i++ {
		if err := checkSigningWidgetAppearance(engine, page, i); err != nil {
			return err
		}
	}
	return nil
}

func checkSigningWidgetAppearance(engine pdfium.Pdfium, page requests.Page, index int) error {
	annotation, err := engine.FPDFPage_GetAnnot(&requests.FPDFPage_GetAnnot{Page: page, Index: index})
	if err != nil {
		return errors.New("PDF annotation could not be read")
	}
	defer engine.FPDFPage_CloseAnnot(&requests.FPDFPage_CloseAnnot{Annotation: annotation.Annotation})
	subtype, err := engine.FPDFAnnot_GetSubtype(&requests.FPDFAnnot_GetSubtype{Annotation: annotation.Annotation})
	if err != nil {
		return errors.New("PDF annotation type could not be read")
	}
	if subtype.Subtype != enums.FPDF_ANNOT_SUBTYPE_WIDGET {
		return nil
	}
	appearance, err := engine.FPDFAnnot_GetAP(&requests.FPDFAnnot_GetAP{Annotation: annotation.Annotation, AppearanceMode: enums.FPDF_ANNOT_APPEARANCEMODE_NORMAL})
	if err != nil || appearance.Value == "" {
		return errors.New("a PDF form field has no usable visible appearance; save it from a PDF editor before uploading")
	}
	return nil
}

func renderSigningFlattenPage(ctx context.Context, engine pdfium.Pdfium, page requests.Page) ([]byte, error) {
	rendered, err := engine.RenderPageInDPI(&requests.RenderPageInDPI{Page: page, DPI: signingFlattenDPI, RenderFlags: enums.FPDF_RENDER_FLAG_ANNOT, RenderForm: true})
	if err != nil {
		return nil, errors.New("PDF page could not be rendered safely")
	}
	defer rendered.Cleanup()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if rendered.Result.Width <= 0 || rendered.Result.Height <= 0 || int64(rendered.Result.Width)*int64(rendered.Result.Height) > signingFlattenMaxPixels {
		return nil, errors.New("rendered PDF page exceeds processing limits")
	}
	img := rendered.Result.RenderedImage
	if img == nil {
		return nil, errors.New("PDF page did not produce an image")
	}
	if rendered.Result.HasTransparency {
		white := image.NewRGBA(img.Bounds())
		draw.Draw(white, white.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.Draw(white, white.Bounds(), img, img.Bounds().Min, draw.Over)
		img = white
	}
	var output signingFlattenBuffer
	if err := jpeg.Encode(&output, img, &jpeg.Options{Quality: 95}); err != nil {
		return nil, errors.New("PDF page image exceeds preparation limits")
	}
	return output.Bytes(), nil
}

type signingFlattenBuffer struct{ bytes.Buffer }

func (b *signingFlattenBuffer) Write(data []byte) (int, error) {
	if len(data) > signingPDFMaxBytes-b.Len() {
		return 0, fmt.Errorf("prepared PDF exceeds %d bytes", signingPDFMaxBytes)
	}
	return b.Buffer.Write(data)
}
