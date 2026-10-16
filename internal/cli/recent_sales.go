// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
//
// `recent-sales list` — live scrape of /recent-sales/. The static page served
// by the site carries no server-rendered gallery (the grid is client-rendered),
// so the honest default is an empty result with a stderr note; any gallery
// markup that does appear is extracted as records.

package cli

import (
	"fmt"

	"ceiba-cli-pp-cli/internal/scrape"
	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newCeibaRecentSalesCmd(flags))
	})
}

func newCeibaRecentSalesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "recent-sales",
		Short:       "Sold-property gallery from /recent-sales/ (client-rendered; best-effort static scrape)",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:parent-group": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newCeibaRecentSalesListCmd(flags))
	return cmd
}

// ceibaRecentSaleView is the machine shape of one recent-sale record.
type ceibaRecentSaleView struct {
	Title    string `json:"title,omitempty"`
	Caption  string `json:"caption,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	URL      string `json:"url,omitempty"`
}

var ceibaRecentSaleKeepKeys = []string{"title", "caption", "image_url", "url"}

func newCeibaRecentSalesListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List recent sales records scraped live from /recent-sales/",
		Example:     "  ceiba-cli-pp-cli recent-sales list",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if len(args) > 0 {
				return usageErr(fmt.Errorf("recent-sales list takes no positional arguments"))
			}
			if dryRunOK(flags) {
				return writeDryRun(w, flags, "recent-sales list (live /recent-sales/ scrape)")
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			html, err := c.GetWithHeaders(ctx, "/recent-sales/", nil, map[string]string{"X-Printing-Press-HTML-Response": "true"})
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			records := scrape.ParseRecentSales(html)
			views := make([]ceibaRecentSaleView, 0, len(records))
			for _, r := range records {
				views = append(views, ceibaRecentSaleView{
					Title:    r.Title,
					Caption:  r.Caption,
					ImageURL: r.ImageURL,
					URL:      r.URL,
				})
			}
			if len(views) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: /recent-sales/ serves no gallery in its static HTML (the grid is client-rendered); no sale records are extractable — an empty result here does not mean there were no sales")
			}
			if wantsHumanTable(w, flags) {
				return printAutoTable(w, ceibaViewsToMaps(views))
			}
			return printJSONFilteredKeep(w, views, flags, ceibaRecentSaleKeepKeys...)
		},
	}
	return cmd
}
