package content

import "testing"

func TestPDFName_IsTheFileInTheAddress(t *testing.T) {
	t.Parallel()
	for rawURL, want := range map[string]string{
		"https://papers.example/raft%20consensus.pdf": "raft consensus",
		"https://papers.example/papers/Raft.PDF":      "Raft",
		"https://papers.example/download/report?id=3": "report",
		"https://papers.example/papers/":              "papers",
		"https://papers.example/":                     "",
		"https://papers.example":                      "",
		"://not a url":                                "",
	} {
		if got := pdfName(rawURL); got != want {
			t.Errorf("pdfName(%q) = %q, want %q", rawURL, got, want)
		}
	}
}
