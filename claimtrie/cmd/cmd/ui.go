package cmd

import (
	"fmt"
	"strings"

	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/claimtrie/node"
)

var status = map[node.Status]string{
	node.Accepted:    "Accepted",
	node.Activated:   "Activated",
	node.Deactivated: "Deactivated",
}

func changeType(c change.ChangeType) string {
	switch c {
	case change.AddClaim:
		return "AddClaim"
	case change.SpendClaim:
		return "SpendClaim"
	case change.UpdateClaim:
		return "UpdateClaim"
	case change.AddSupport:
		return "AddSupport"
	case change.SpendSupport:
		return "SpendSupport"
	}
	return "Unknown"
}

func showChange(chg change.Change) {
	fmt.Printf(">>> Height: %6d: %s for %04s, %15d, %s - %s\n",
		chg.Height, changeType(chg.Type), chg.ClaimID, chg.Amount, chg.OutPoint, chg.Name)
}

func showClaim(c *node.Claim, n *node.Node) {
	mark := " "
	if c == n.BestClaim {
		mark = "*"
	}

	fmt.Printf("%s  C  ID: %s, TXO: %s\n   %5d/%-5d, Status: %9s, Amount: %15d, Support Amount: %15d\n",
		mark, c.ClaimID, c.OutPoint, c.AcceptedAt, c.ActiveAt, status[c.Status], c.Amount, n.SupportSums[c.ClaimID.Key()])
}

func showSupport(c *node.Claim) {
	fmt.Printf("    S id: %s, op: %s, %5d/%-5d, %9s, amt: %15d\n",
		c.ClaimID, c.OutPoint, c.AcceptedAt, c.ActiveAt, status[c.Status], c.Amount)
}

func showNode(n *node.Node) {

	fmt.Printf("%s\n", strings.Repeat("-", 200))
	fmt.Printf("Last Node Takeover: %d\n\n", n.TakenOverAt)
	n.SortClaimsByBid()
	for _, c := range n.Claims {
		showClaim(c, n)
		for _, s := range n.Supports {
			if s.ClaimID != c.ClaimID {
				continue
			}
			showSupport(s)
		}
	}
	fmt.Printf("\n\n")
}

func showTemporalNames(height int32, names [][]byte) {
	fmt.Printf("%7d: %q", height, names[0])
	for _, name := range names[1:] {
		fmt.Printf(", %q ", name)
	}
	fmt.Printf("\n")
}
