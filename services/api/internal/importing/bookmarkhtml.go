package importing

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ParseHTML extracts bookmark records from a browser export in the Netscape
// bookmark format (Chrome, Firefox, Safari, Edge).
//
// The input is treated as untrusted data: it is tokenized, never rendered,
// and the content of <script> and <style> elements is ignored. Folder paths
// come from <H3> headings followed by a nested <DL>. Malformed entries are
// returned as-is so validation can report them per record.
func ParseHTML(r io.Reader, maxRecords int) ([]Record, error) {
	p := htmlParser{maxRecords: maxRecords, records: []Record{}}
	z := html.NewTokenizer(r)

	for {
		switch z.Next() {
		case html.ErrorToken:
			if err := z.Err(); !errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("reading html: %w", err)
			}
			if err := p.closeAnchor(); err != nil {
				return nil, err
			}
			return p.records, nil
		case html.StartTagToken:
			if err := p.startTag(z.Token()); err != nil {
				return nil, err
			}
		case html.EndTagToken:
			if err := p.endTag(z.Token()); err != nil {
				return nil, err
			}
		case html.TextToken:
			p.text(string(z.Text()))
		default:
		}
	}
}

type folderLevel struct {
	name    string
	isNamed bool
	// isRoot marks a browser's own folder (bookmarks bar, other bookmarks),
	// which never becomes a collection.
	isRoot bool
}

type htmlParser struct {
	maxRecords int
	records    []Record

	folders       []folderLevel
	pendingFolder *string
	pendingIsRoot bool
	headingIsRoot bool

	isInHeading bool
	heading     strings.Builder

	isInRawText bool

	anchor      *Record
	isInA       bool
	anchorTitle strings.Builder
}

func (p *htmlParser) startTag(tok html.Token) error {
	switch tok.Data {
	case "script", "style":
		p.isInRawText = true
	case "h3":
		p.isInHeading = true
		p.heading.Reset()
		p.headingIsRoot = slices.ContainsFunc(tok.Attr, func(a html.Attribute) bool {
			return (a.Key == "personal_toolbar_folder" || a.Key == "unfiled_bookmarks_folder") && strings.EqualFold(a.Val, "true")
		})
	case "dl":
		if err := p.closeAnchor(); err != nil {
			return err
		}
		level := folderLevel{}
		if p.pendingFolder != nil {
			level = folderLevel{name: *p.pendingFolder, isNamed: true, isRoot: p.pendingIsRoot}
		}
		p.folders = append(p.folders, level)
		p.pendingFolder = nil
		p.pendingIsRoot = false
	case "dt":
		return p.closeAnchor()
	case "a":
		if err := p.closeAnchor(); err != nil {
			return err
		}
		p.openAnchor(tok.Attr)
	default:
	}
	return nil
}

func (p *htmlParser) endTag(tok html.Token) error {
	switch tok.Data {
	case "script", "style":
		p.isInRawText = false
	case "h3":
		name := collapseSpace(p.heading.String())
		p.pendingFolder = &name
		p.pendingIsRoot = p.headingIsRoot
		p.isInHeading = false
	case "dl":
		if err := p.closeAnchor(); err != nil {
			return err
		}
		if len(p.folders) > 0 {
			p.folders = p.folders[:len(p.folders)-1]
		}
		p.pendingFolder = nil
	case "a":
		return p.closeAnchor()
	default:
	}
	return nil
}

func (p *htmlParser) text(s string) {
	switch {
	case p.isInRawText:
	case p.isInA:
		p.anchorTitle.WriteString(s)
	case p.isInHeading:
		p.heading.WriteString(s)
	}
}

func (p *htmlParser) openAnchor(attrs []html.Attribute) {
	rec := Record{Folder: p.folderPath(), Collection: p.collection()}
	for _, a := range attrs {
		switch a.Key {
		case "href":
			rec.URL = strings.TrimSpace(a.Val)
		case "add_date":
			rec.AddedAt = parseUnixDate(a.Val)
		case "tags":
			rec.Tags = splitTags(a.Val)
		default:
		}
	}
	p.anchor = &rec
	p.isInA = true
	p.anchorTitle.Reset()
}

func (p *htmlParser) closeAnchor() error {
	if p.anchor == nil {
		return nil
	}
	if len(p.records) >= p.maxRecords {
		return fmt.Errorf("%w: limit is %d", ErrTooManyRecords, p.maxRecords)
	}

	rec := *p.anchor
	rec.Title = collapseSpace(p.anchorTitle.String())
	rec.Position = len(p.records) + 1
	p.records = append(p.records, rec)

	p.anchor = nil
	p.isInA = false
	return nil
}

func (p *htmlParser) folderPath() []string {
	path := []string{}
	for _, f := range p.folders {
		if f.isNamed {
			path = append(path, f.name)
		}
	}
	return path
}

// collection is the first named folder that is not a browser root folder,
// or "" when the bookmark sits directly in a root folder or in none.
func (p *htmlParser) collection() string {
	for _, f := range p.folders {
		if f.isNamed && !f.isRoot {
			return f.name
		}
	}
	return ""
}

// parseUnixDate accepts seconds, milliseconds or microseconds since the epoch,
// as different browsers export them. Invalid values are ignored.
func parseUnixDate(v string) *time.Time {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n <= 0 {
		return nil
	}

	var t time.Time
	switch {
	case n > 1e14:
		t = time.UnixMicro(n)
	case n > 1e11:
		t = time.UnixMilli(n)
	default:
		t = time.Unix(n, 0)
	}
	t = t.UTC()
	return &t
}

func splitTags(v string) []string {
	var tags []string
	for tag := range strings.SplitSeq(v, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
