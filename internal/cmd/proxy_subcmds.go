package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// AnnotationPolecatSafe marks a cobra command as safe for polecat sandbox execution.
// Commands with this annotation are included in the output of "gt proxy-subcmds".
// Add Annotations: map[string]string{AnnotationPolecatSafe: "true"} to any
// gt subcommand that polecats should be permitted to run through the proxy.
const AnnotationPolecatSafe = "polecatSafe"

// bdSafeSubcmds lists the bd subcommands safe for polecat sandbox execution.
// Unlike gt subcommands (which are auto-discovered via AnnotationPolecatSafe),
// bd subcommands are listed here since bd does not embed annotations.
const bdSafeSubcmds = "create,update,close,show,list,ready,dep,export,prime,stats,blocked,doctor"

var proxySubcmdsCmd = &cobra.Command{
	Use:    "proxy-subcmds",
	Hidden: true,
	Short:  "Output allowed subcommands for the proxy server",
	Long: `Output the allowed subcommand allowlist for gt-proxy-server.

Prints a semicolon-separated "cmd:sub1,sub2,..." string listing which
subcommands polecats may invoke through the mTLS proxy. The gt portion
is discovered automatically by scanning commands annotated with the
polecatSafe annotation; the bd portion is a fixed list embedded here.

The proxy server calls this command at startup and falls back to its
built-in default if discovery fails.`,
	Run: func(cmd *cobra.Command, args []string) {
		var gtSubs []string
		for _, c := range rootCmd.Commands() {
			if c.Annotations[AnnotationPolecatSafe] == "true" {
				gtSubs = append(gtSubs, c.Name())
			}
		}
		sort.Strings(gtSubs)
		fmt.Printf("gt:%s;bd:%s\n", strings.Join(gtSubs, ","), bdSafeSubcmds)
	},
}

func init() {
	rootCmd.AddCommand(proxySubcmdsCmd)
}
