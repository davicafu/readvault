//go:build integration

package content_test

import (
	"bytes"
	"errors"
	"testing"

	"readvault/api/internal/content"
)

const pdfURL = "https://papers.example/raft%20consensus.pdf"

// pdfFile is a PDF larger than setup's 64 KiB page limit: a PDF has its own.
var pdfFile = append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("0"), 100<<10)...)

func TestFetch_KeepsAPDFAsItIs(t *testing.T) {
	t.Parallel()
	for _, contentType := range []string{"application/pdf", "application/octet-stream"} {
		t.Run(contentType, func(t *testing.T) {
			t.Parallel()
			svc, f, pool, _ := setup(t)
			id := newArticle(t, pool, owner, pdfURL)
			if _, err := pool.Exec(t.Context(), `UPDATE articles SET title = original_url WHERE id = $1`, id); err != nil {
				t.Fatal(err)
			}
			f.responses[pdfURL] = content.Fetched{ContentType: contentType, Body: pdfFile}

			got, err := svc.Fetch(t.Context(), owner, id)
			if err != nil {
				t.Fatalf("Fetch() error: %v", err)
			}
			if !got.PDF || got.Status != content.StatusComplete || got.Markdown != "" || got.Words != 0 {
				t.Errorf("content = pdf %v, status %q, markdown %q, words %d; want a complete PDF without text", got.PDF, got.Status, got.Markdown, got.Words)
			}
			pdf, err := svc.PDF(t.Context(), owner, id)
			if err != nil || !bytes.Equal(pdf.Data, pdfFile) || pdf.Name != "raft consensus.pdf" {
				t.Errorf("PDF() = name %q, %d bytes, err %v; want the file as downloaded", pdf.Name, len(pdf.Data), err)
			}
			var title string
			if err := pool.QueryRow(t.Context(), `SELECT title FROM articles WHERE id = $1`, id).Scan(&title); err != nil || title != "raft consensus" {
				t.Errorf("title %q (err %v), want the file's name", title, err)
			}
		})
	}
}

func TestFetch_AnHTMLPageReplacesASavedPDF(t *testing.T) {
	t.Parallel()
	svc, f, _, id := setup(t)
	f.responses[articleURL] = content.Fetched{ContentType: "application/pdf", Body: pdfFile}
	if _, err := svc.Fetch(t.Context(), owner, id); err != nil {
		t.Fatal(err)
	}
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(articlePage)}

	got, err := svc.Fetch(t.Context(), owner, id)
	if err != nil || got.PDF || got.Words == 0 {
		t.Fatalf("Fetch() = pdf %v, words %d, err %v; want the page's text", got.PDF, got.Words, err)
	}
	if _, err := svc.PDF(t.Context(), owner, id); !errors.Is(err, content.ErrNoContent) {
		t.Errorf("PDF() error = %v, want ErrNoContent", err)
	}
}

func TestFetch_AnHTMLPageOverThePageLimitIsTooLarge(t *testing.T) {
	t.Parallel()
	svc, f, _, id := setup(t)
	big := "<html><body><article><p>" + string(bytes.Repeat([]byte("word "), 20<<10)) + "</p></article></body></html>"
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(big)}

	_, err := svc.Fetch(t.Context(), owner, id)
	var fe *content.FetchError
	if !errors.As(err, &fe) || fe.Problem != content.ProblemTooLarge {
		t.Errorf("Fetch() error = %v, want %s", err, content.ProblemTooLarge)
	}
}

func TestPDF_WithoutASavedPDFIsNoContent(t *testing.T) {
	t.Parallel()
	svc, f, _, id := setup(t)
	if _, err := svc.PDF(t.Context(), owner, id); !errors.Is(err, content.ErrNoContent) {
		t.Errorf("before any download: PDF() error = %v, want ErrNoContent", err)
	}
	f.responses[articleURL] = content.Fetched{ContentType: "text/html", Body: []byte(articlePage)}
	if _, err := svc.Fetch(t.Context(), owner, id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PDF(t.Context(), owner, id); !errors.Is(err, content.ErrNoContent) {
		t.Errorf("with a text: PDF() error = %v, want ErrNoContent", err)
	}
	if _, err := svc.PDF(t.Context(), owner, "not-a-uuid"); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("unknown article: PDF() error = %v, want ErrNotFound", err)
	}
}

func TestFetch_AFileSentAsAPDFThatIsNotOneIsNotHTML(t *testing.T) {
	t.Parallel()
	for _, contentType := range []string{"application/pdf", "application/octet-stream"} {
		t.Run(contentType, func(t *testing.T) {
			t.Parallel()
			svc, f, _, id := setup(t)
			f.responses[articleURL] = content.Fetched{ContentType: contentType, Body: []byte(articlePage)}

			_, err := svc.Fetch(t.Context(), owner, id)
			var fe *content.FetchError
			if !errors.As(err, &fe) || fe.Problem != content.ProblemNotHTML {
				t.Fatalf("Fetch() error = %v, want %s", err, content.ProblemNotHTML)
			}
			if _, err := svc.Get(t.Context(), owner, id); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.PDF(t.Context(), owner, id); !errors.Is(err, content.ErrNoContent) {
				t.Errorf("PDF() error = %v, want nothing saved", err)
			}
		})
	}
}
