// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
//
// `developments list` reads the synced `development` resource rows from the
// local mirror; `developments show <id|slug>` scrapes the live
// /development/<slug>/ page for description and gallery images (best-effort)
// with the REST resource as fallback.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"ceiba-cli-pp-cli/internal/cliutil"
	"ceiba-cli-pp-cli/internal/scrape"
	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newCeibaDevelopmentsCmd(flags))
	})
}

func newCeibaDevelopmentsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "developments",
		Short:       "Pre-built community developments: local list + live detail pages",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:parent-group": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newCeibaDevelopmentsListCmd(flags))
	cmd.AddCommand(newCeibaDevelopmentsShowCmd(flags))
	return cmd
}

// ceibaDevelopmentRow is one mirrored development.
type ceibaDevelopmentRow struct {
	ID       int64  `json:"id"`
	Slug     string `json:"slug,omitempty"`
	Title    string `json:"title,omitempty"`
	Modified string `json:"modified,omitempty"`
	Link     string `json:"link,omitempty"`
}

var ceibaDevelopmentKeepKeys = []string{"id", "slug", "title", "modified", "link"}

// pp:data-source local
func newCeibaDevelopmentsListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List mirrored developments (run 'sync' first)",
		Example:     "  ceiba-cli-pp-cli developments list",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if len(args) > 0 {
				return usageErr(fmt.Errorf("developments list takes no positional arguments"))
			}
			if dryRunOK(flags) {
				return writeDryRun(w, flags, "developments list (local development mirror)")
			}
			ctx := cmd.Context()
			db, err := openStoreForRead(ctx, ceibaStoreName)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			if db == nil {
				return writeCeibaEmptyMirror(w, flags, "the local developments mirror")
			}
			defer db.Close()

			raw, err := db.List("development", 0)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			rows := make([]ceibaDevelopmentRow, 0, len(raw))
			for _, item := range raw {
				var parsed struct {
					ID       int64  `json:"id"`
					Slug     string `json:"slug"`
					Modified string `json:"modified"`
					Link     string `json:"link"`
					Title    struct {
						Rendered string `json:"rendered"`
					} `json:"title"`
				}
				if json.Unmarshal(item, &parsed) != nil || parsed.ID == 0 {
					continue
				}
				rows = append(rows, ceibaDevelopmentRow{
					ID:       parsed.ID,
					Slug:     parsed.Slug,
					Title:    cliutil.CleanText(parsed.Title.Rendered),
					Modified: parsed.Modified,
					Link:     parsed.Link,
				})
			}
			if len(rows) == 0 {
				return writeCeibaEmptyMirror(w, flags, "the local developments mirror")
			}
			if wantsHumanTable(w, flags) {
				items := ceibaViewsToMaps(rows)
				return printAutoTable(w, items)
			}
			return printJSONFilteredKeep(w, rows, flags, ceibaDevelopmentKeepKeys...)
		},
	}
	return cmd
}

type ceibaDevelopmentPage struct {
	Slug        string   `json:"slug"`
	ID          int64    `json:"id,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Images      []string `json:"images,omitempty"`
	Link        string   `json:"link,omitempty"`
	Modified    string   `json:"modified,omitempty"`
	SourceURL   string   `json:"source_url,omitempty"`
}

// pp:data-source live
func newCeibaDevelopmentsShowCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "show <id|slug>",
		Short:       "Live development detail: description and gallery images scraped from /development/<slug>/",
		Example:     "  ceiba-cli-pp-cli developments show hopkins-grove",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "id=hopkins-grove"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				action := "developments show"
				if len(args) > 0 {
					action += " " + strings.Join(args, " ")
				}
				return writeDryRun(w, flags, action)
			}
			if len(args) < 1 {
				return usageErr(fmt.Errorf("developments show requires a development id or slug"))
			}
			if len(args) > 1 {
				return usageErr(fmt.Errorf("developments show takes exactly one development reference, got %d", len(args)))
			}
			key := strings.TrimSpace(args[0])
			ctx := cmd.Context()

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			page, err := ceibaDevelopmentShow(ctx, c, key, cmd.ErrOrStderr())
			if err != nil {
				var typed *cliError
				if As(err, &typed) {
					writeAPIErrorEnvelope(w, flags, err, ExitCode(err))
					return err
				}
				return classifyAPIError(w, err, flags)
			}
			if wantsHumanTable(w, flags) {
				return printAutoTable(w, ceibaViewsToMaps([]ceibaDevelopmentPage{page}))
			}
			payload := map[string]any{
				"results": page,
				"meta":    map[string]any{"source": "live"},
			}
			return printJSONFiltered(w, payload, flags)
		},
	}
	return cmd
}

func ceibaDevelopmentShow(ctx context.Context, c interface {
	GetWithHeaders(ctx context.Context, path string, params map[string]string, headers map[string]string) (json.RawMessage, error)
}, key string, warn io.Writer) (ceibaDevelopmentPage, error) {
	var (
		page ceibaDevelopmentPage
		slug = key
	)
	if n, err := strconv.ParseInt(key, 10, 64); err == nil && n > 0 {
		raw, err := c.GetWithHeaders(ctx, fmt.Sprintf("/wp-json/wp/v2/development/%d", n), nil, nil)
		if err != nil {
			if strings.Contains(err.Error(), "HTTP 404") {
				return page, notFoundErr(fmt.Errorf("development id %d not found on ceibabelize.com", n))
			}
			return page, err
		}
		var item struct {
			ID    int64  `json:"id"`
			Slug  string `json:"slug"`
			Link  string `json:"link"`
			Title struct {
				Rendered string `json:"rendered"`
			} `json:"title"`
		}
		if json.Unmarshal(raw, &item) != nil || item.ID == 0 {
			return page, notFoundErr(fmt.Errorf("development id %d not found on ceibabelize.com", n))
		}
		slug = item.Slug
		page.ID = item.ID
		page.Link = item.Link
		page.Title = cliutil.CleanText(item.Title.Rendered)
	}

	page.Slug = slug
	page.SourceURL = "https://ceibabelize.com/development/" + slug + "/"

	html, err := c.GetWithHeaders(ctx, "/development/"+cliutil.EscapePathParam(slug)+"/", nil, map[string]string{"X-Printing-Press-HTML-Response": "true"})
	if err != nil {
		if page.ID == 0 {
			// scrape failed and no REST identity yet: try the REST list by slug.
			rest, restErr := ceibaDevelopmentRESTBySlug(ctx, c, slug)
			if restErr != nil {
				return page, restErr
			}
			if rest.ID == 0 && rest.Title == "" {
				return page, notFoundErr(fmt.Errorf("development %q not found on ceibabelize.com (detail page fetch failed: %v)", key, err))
			}
			return rest, nil
		}
		// REST identity known: degrade to REST-only payload with a warning.
		fmt.Fprintf(warn, "warning: detail page fetch failed (%v); serving REST fields only\n", err)
		if page.Link == "" {
			page.Link = "https://ceibabelize.com/development/" + slug + "/"
		}
		return page, nil
	}

	scraped := scrape.ParseDevelopmentPage(html)
	if page.Title == "" {
		page.Title = scraped.Title
	}
	page.Description = scraped.Description
	page.Images = scraped.Images
	if page.ID == 0 {
		if rest, restErr := ceibaDevelopmentRESTBySlug(ctx, c, slug); restErr == nil && (rest.ID != 0 || rest.Title != "") {
			page.ID = rest.ID
			page.Modified = rest.Modified
			if page.Link == "" {
				page.Link = rest.Link
			}
		}
	}
	if page.Title == "" && len(page.Images) == 0 {
		return page, notFoundErr(fmt.Errorf("development %q not found on ceibabelize.com (no page content and no REST match)", key))
	}
	return page, nil
}

func ceibaDevelopmentRESTBySlug(ctx context.Context, c interface {
	GetWithHeaders(ctx context.Context, path string, params map[string]string, headers map[string]string) (json.RawMessage, error)
}, slug string) (ceibaDevelopmentPage, error) {
	raw, err := c.GetWithHeaders(ctx, "/wp-json/wp/v2/development", map[string]string{"slug": slug}, nil)
	if err != nil {
		return ceibaDevelopmentPage{}, err
	}
	var items []struct {
		ID       int64  `json:"id"`
		Slug     string `json:"slug"`
		Modified string `json:"modified"`
		Link     string `json:"link"`
		Title    struct {
			Rendered string `json:"rendered"`
		} `json:"title"`
	}
	if json.Unmarshal(raw, &items) != nil || len(items) == 0 {
		return ceibaDevelopmentPage{
			Slug:      slug,
			SourceURL: "https://ceibabelize.com/development/" + slug + "/",
		}, nil
	}
	item := items[0]
	return ceibaDevelopmentPage{
		Slug:      item.Slug,
		ID:        item.ID,
		Title:     cliutil.CleanText(item.Title.Rendered),
		Link:      item.Link,
		Modified:  item.Modified,
		SourceURL: "https://ceibabelize.com/development/" + item.Slug + "/",
	}, nil
}
