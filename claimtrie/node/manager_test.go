package node

import (
	"fmt"
	"testing"

	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/claimtrie/node/noderepo"
	"github.com/lbryio/lbcd/claimtrie/param"
	"github.com/lbryio/lbcd/wire"

	"github.com/stretchr/testify/require"
)

var (
	out1  = NewOutPointFromString("0000000000000000000000000000000000000000000000000000000000000000:1")
	out2  = NewOutPointFromString("0000000000000000000000000000000000000000000000000000000000000000:2")
	out3  = NewOutPointFromString("0100000000000000000000000000000000000000000000000000000000000000:1")
	out4  = NewOutPointFromString("0100000000000000000000000000000000000000000000000000000000000000:2")
	name1 = []byte("name1")
	name2 = []byte("name2")
)

// verify that we can round-trip bytes to strings
func TestStringRoundTrip(t *testing.T) {

	r := require.New(t)

	data := [][]byte{
		{97, 98, 99, 0, 100, 255},
		{0xc3, 0x28},
		{0xa0, 0xa1},
		{0xe2, 0x28, 0xa1},
		{0xf0, 0x28, 0x8c, 0x28},
	}
	for _, d := range data {
		s := string(d)
		r.Equal(s, fmt.Sprintf("%s", d)) // nolint
		d2 := []byte(s)
		r.Equal(len(d), len(s))
		r.Equal(d, d2)
	}
}

func TestSimpleAddClaim(t *testing.T) {

	r := require.New(t)

	param.SetNetwork(wire.TestNet)
	repo, err := noderepo.NewPebble(t.TempDir())
	r.NoError(err)

	m, err := NewBaseManager(repo)
	r.NoError(err)
	defer m.Close()

	_, err = m.IncrementHeightTo(10, false)
	r.NoError(err)

	chg := change.NewChange(change.AddClaim).SetName(name1).SetOutPoint(out1).SetHeight(11)
	m.AppendChange(chg)
	_, err = m.IncrementHeightTo(11, false)
	r.NoError(err)

	chg = chg.SetName(name2).SetOutPoint(out2).SetHeight(12)
	m.AppendChange(chg)
	_, err = m.IncrementHeightTo(12, false)
	r.NoError(err)

	n1, err := m.node(name1)
	r.NoError(err)
	r.Equal(1, len(n1.Claims))
	r.NotNil(n1.Claims.find(byOut(*out1)))

	n2, err := m.node(name2)
	r.NoError(err)
	r.Equal(1, len(n2.Claims))
	r.NotNil(n2.Claims.find(byOut(*out2)))

	_, err = m.DecrementHeightTo([][]byte{name2}, 11)
	r.NoError(err)
	n2, err = m.node(name2)
	r.NoError(err)
	r.Nil(n2)

	_, err = m.DecrementHeightTo([][]byte{name1}, 1)
	r.NoError(err)
	n2, err = m.node(name1)
	r.NoError(err)
	r.Nil(n2)
}

func TestSupportAmounts(t *testing.T) {

	r := require.New(t)

	param.SetNetwork(wire.TestNet)
	repo, err := noderepo.NewPebble(t.TempDir())
	r.NoError(err)

	m, err := NewBaseManager(repo)
	r.NoError(err)
	defer m.Close()

	_, err = m.IncrementHeightTo(10, false)
	r.NoError(err)

	chg := change.NewChange(change.AddClaim).SetName(name1).SetOutPoint(out1).SetHeight(11).SetAmount(3)
	chg.ClaimID = change.NewClaimID(*out1)
	m.AppendChange(chg)

	chg = change.NewChange(change.AddClaim).SetName(name1).SetOutPoint(out2).SetHeight(11).SetAmount(4)
	chg.ClaimID = change.NewClaimID(*out2)
	m.AppendChange(chg)

	_, err = m.IncrementHeightTo(11, false)
	r.NoError(err)

	chg = change.NewChange(change.AddSupport).SetName(name1).SetOutPoint(out3).SetHeight(12).SetAmount(2)
	chg.ClaimID = change.NewClaimID(*out1)
	m.AppendChange(chg)

	chg = change.NewChange(change.AddSupport).SetName(name1).SetOutPoint(out4).SetHeight(12).SetAmount(2)
	chg.ClaimID = change.NewClaimID(*out2)
	m.AppendChange(chg)

	chg = change.NewChange(change.SpendSupport).SetName(name1).SetOutPoint(out4).SetHeight(12).SetAmount(2)
	chg.ClaimID = change.NewClaimID(*out2)
	m.AppendChange(chg)

	_, err = m.IncrementHeightTo(20, false)
	r.NoError(err)

	n1, err := m.node(name1)
	r.NoError(err)
	r.Equal(2, len(n1.Claims))
	r.Equal(int64(5), n1.BestClaim.Amount+n1.SupportSums[n1.BestClaim.ClaimID.Key()])
}

func TestNodeSort(t *testing.T) {

	r := require.New(t)

	param.ActiveParams.ExtendedClaimExpirationTime = 1000

	r.True(OutPointLess(*out1, *out2))
	r.True(OutPointLess(*out1, *out3))

	n := New()
	n.Claims = append(n.Claims, &Claim{OutPoint: *out1, AcceptedAt: 3, Amount: 3, ClaimID: change.ClaimID{1}})
	n.Claims = append(n.Claims, &Claim{OutPoint: *out2, AcceptedAt: 3, Amount: 3, ClaimID: change.ClaimID{2}})
	n.handleExpiredAndActivated(3)
	n.updateTakeoverHeight(3, []byte{}, true)

	r.Equal(n.Claims.find(byOut(*out1)).OutPoint.String(), n.BestClaim.OutPoint.String())

	n.Claims = append(n.Claims, &Claim{OutPoint: *out3, AcceptedAt: 3, Amount: 3, ClaimID: change.ClaimID{3}})
	n.handleExpiredAndActivated(3)
	n.updateTakeoverHeight(3, []byte{}, true)
	r.Equal(n.Claims.find(byOut(*out1)).OutPoint.String(), n.BestClaim.OutPoint.String())
}

func TestClaimSort(t *testing.T) {

	r := require.New(t)

	param.ActiveParams.ExtendedClaimExpirationTime = 1000

	n := New()
	n.Claims = append(n.Claims, &Claim{OutPoint: *out2, AcceptedAt: 3, Amount: 3, ClaimID: change.ClaimID{2}, Status: Activated})
	n.Claims = append(n.Claims, &Claim{OutPoint: *out3, AcceptedAt: 3, Amount: 2, ClaimID: change.ClaimID{3}, Status: Activated})
	n.Claims = append(n.Claims, &Claim{OutPoint: *out3, AcceptedAt: 4, Amount: 2, ClaimID: change.ClaimID{4}, Status: Activated})
	n.Claims = append(n.Claims, &Claim{OutPoint: *out1, AcceptedAt: 3, Amount: 4, ClaimID: change.ClaimID{1}, Status: Activated})
	n.Claims = append(n.Claims, &Claim{OutPoint: *out1, AcceptedAt: 1, Amount: 9, ClaimID: change.ClaimID{5}, Status: Accepted})
	n.SortClaimsByBid()

	r.Equal(int64(4), n.Claims[0].Amount)
	r.Equal(int64(3), n.Claims[1].Amount)
	r.Equal(int64(2), n.Claims[2].Amount)
	r.Equal(int32(4), n.Claims[3].AcceptedAt)
}

func TestHasChildren(t *testing.T) {
	r := require.New(t)

	param.SetNetwork(wire.TestNet)
	repo, err := noderepo.NewPebble(t.TempDir())
	r.NoError(err)

	m, err := NewBaseManager(repo)
	r.NoError(err)
	defer m.Close()

	chg := change.NewChange(change.AddClaim).SetName([]byte("a")).SetOutPoint(out1).SetHeight(1).SetAmount(2)
	chg.ClaimID = change.NewClaimID(*out1)
	m.AppendChange(chg)
	_, err = m.IncrementHeightTo(1, false)
	r.NoError(err)
	r.False(m.hasChildren([]byte("a"), 1, nil, 1))

	chg = change.NewChange(change.AddClaim).SetName([]byte("ab")).SetOutPoint(out2).SetHeight(2).SetAmount(2)
	chg.ClaimID = change.NewClaimID(*out2)
	m.AppendChange(chg)
	_, err = m.IncrementHeightTo(2, false)
	r.NoError(err)
	r.False(m.hasChildren([]byte("a"), 2, nil, 2))
	r.True(m.hasChildren([]byte("a"), 2, nil, 1))

	chg = change.NewChange(change.AddClaim).SetName([]byte("abc")).SetOutPoint(out3).SetHeight(3).SetAmount(2)
	chg.ClaimID = change.NewClaimID(*out3)
	m.AppendChange(chg)
	_, err = m.IncrementHeightTo(3, false)
	r.NoError(err)
	r.False(m.hasChildren([]byte("a"), 3, nil, 2))

	chg = change.NewChange(change.AddClaim).SetName([]byte("ac")).SetOutPoint(out1).SetHeight(4).SetAmount(2)
	chg.ClaimID = change.NewClaimID(*out4)
	m.AppendChange(chg)
	_, err = m.IncrementHeightTo(4, false)
	r.NoError(err)
	r.True(m.hasChildren([]byte("a"), 4, nil, 2))
}

func TestCollectChildren(t *testing.T) {
	r := require.New(t)

	c1 := change.Change{Name: []byte("ba"), Type: change.SpendClaim}
	c2 := change.Change{Name: []byte("ba"), Type: change.UpdateClaim}
	c3 := change.Change{Name: []byte("ac"), Type: change.SpendClaim}
	c4 := change.Change{Name: []byte("ac"), Type: change.UpdateClaim}
	c5 := change.Change{Name: []byte("a"), Type: change.SpendClaim}
	c6 := change.Change{Name: []byte("a"), Type: change.UpdateClaim}
	c7 := change.Change{Name: []byte("ab"), Type: change.SpendClaim}
	c8 := change.Change{Name: []byte("ab"), Type: change.UpdateClaim}
	c := []change.Change{c1, c2, c3, c4, c5, c6, c7, c8}

	collectChildNames(c)

	r.Empty(c[0].SpentChildren)
	r.Empty(c[2].SpentChildren)
	r.Empty(c[4].SpentChildren)
	r.Empty(c[6].SpentChildren)

	r.Len(c[1].SpentChildren, 0)
	r.Len(c[3].SpentChildren, 0)
	r.Len(c[5].SpentChildren, 1)
	r.True(c[5].SpentChildren["ac"])

	r.Len(c[7].SpentChildren, 0)
}

func TestTemporaryAddClaim(t *testing.T) {

	r := require.New(t)

	param.SetNetwork(wire.TestNet)
	repo, err := noderepo.NewPebble(t.TempDir())
	r.NoError(err)

	m, err := NewBaseManager(repo)
	r.NoError(err)
	defer m.Close()

	_, err = m.IncrementHeightTo(10, false)
	r.NoError(err)

	chg := change.NewChange(change.AddClaim).SetName(name1).SetOutPoint(out1).SetHeight(11)
	m.AppendChange(chg)
	_, err = m.IncrementHeightTo(11, false)
	r.NoError(err)

	chg = chg.SetName(name2).SetOutPoint(out2).SetHeight(12)
	m.AppendChange(chg)
	_, err = m.IncrementHeightTo(12, true)
	r.NoError(err)

	n1, err := m.node(name1)
	r.NoError(err)
	r.Equal(1, len(n1.Claims))
	r.NotNil(n1.Claims.find(byOut(*out1)))

	n2, err := m.node(name2)
	r.NoError(err)
	r.Equal(1, len(n2.Claims))
	r.NotNil(n2.Claims.find(byOut(*out2)))

	names, err := m.DecrementHeightTo([][]byte{name2}, 11)
	r.Equal(names[0], name2)
	r.NoError(err)
	n2, err = m.node(name2)
	r.NoError(err)
	r.Nil(n2)

	_, err = m.DecrementHeightTo([][]byte{name1}, 1)
	r.NoError(err)
	n2, err = m.node(name1)
	r.NoError(err)
	r.Nil(n2)
}
