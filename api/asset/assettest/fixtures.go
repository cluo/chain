package assettest

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"

	"chain/api/appdb"
	"chain/api/asset"
	"chain/api/txbuilder"
	"chain/database/pg/pgtest"
	"chain/fedchain-sandbox/hdkey"
	"chain/fedchain/bc"
	"chain/fedchain/state"
	"chain/testutil"
)

var userCounter = createCounter()

func CreateUserFixture(ctx context.Context, t testing.TB, email, password string) string {
	if email == "" {
		email = fmt.Sprintf("user-%d@domain.tld", <-userCounter)
	}
	if password == "" {
		password = "drowssap"
	}
	user, err := appdb.CreateUser(ctx, email, password)
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return user.ID
}

func CreateAuthTokenFixture(ctx context.Context, t testing.TB, userID string, typ string, expiresAt *time.Time) *appdb.AuthToken {
	token, err := appdb.CreateAuthToken(ctx, userID, typ, expiresAt)
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return token
}

var projCounter = createCounter()

func CreateProjectFixture(ctx context.Context, t testing.TB, userID, name string) string {
	if userID == "" {
		userID = CreateUserFixture(ctx, t, "", "")
	}
	if name == "" {
		name = fmt.Sprintf("proj-%d", <-projCounter)
	}
	proj, err := appdb.CreateProject(ctx, name, userID)
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return proj.ID
}

func CreateMemberFixture(ctx context.Context, t testing.TB, userID, projectID, role string) {
	if err := appdb.AddMember(ctx, projectID, userID, role); err != nil {
		testutil.FatalErr(t, err)
	}
}

func CreateInvitationFixture(ctx context.Context, t testing.TB, projectID, email, role string) string {
	invitation, err := appdb.CreateInvitation(ctx, projectID, email, role)
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return invitation.ID
}

var issuerNodeCounter = createCounter()

func CreateIssuerNodeFixture(ctx context.Context, t testing.TB, projectID, label string, xpubs, xprvs []*hdkey.XKey) string {
	if projectID == "" {
		projectID = CreateProjectFixture(ctx, t, "", "")
	}
	if label == "" {
		label = fmt.Sprintf("inode-%d", <-issuerNodeCounter)
	}
	if len(xpubs) == 0 && len(xprvs) == 0 {
		xpubs = append(xpubs, testutil.TestXPub)
		xprvs = append(xprvs, testutil.TestXPrv)
	}
	issuerNode, err := appdb.InsertIssuerNode(ctx, projectID, label, xpubs, xprvs, 1)
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return issuerNode.ID
}

func CreateArchivedIssuerNodeFixture(ctx context.Context, t testing.TB, projectID, label string, xpubs, xprvs []*hdkey.XKey) string {
	inodeID := CreateIssuerNodeFixture(ctx, t, projectID, label, xpubs, xprvs)
	err := appdb.ArchiveIssuerNode(ctx, inodeID)
	if err != nil {
		testutil.FatalErr(t, err)
	}

	return inodeID
}

var managerNodeCounter = createCounter()

func CreateManagerNodeFixture(ctx context.Context, t testing.TB, projectID, label string, xpubs, xprvs []*hdkey.XKey) string {
	if projectID == "" {
		projectID = CreateProjectFixture(ctx, t, "", "")
	}
	if label == "" {
		label = fmt.Sprintf("mnode-%d", <-managerNodeCounter)
	}
	if len(xpubs) == 0 && len(xprvs) == 0 {
		xpubs = append(xpubs, testutil.TestXPub)
		xprvs = append(xprvs, testutil.TestXPrv)
	}
	managerNode, err := appdb.InsertManagerNode(ctx, projectID, label, xpubs, xprvs, 0, 1)
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return managerNode.ID
}

var accountCounter = createCounter()

func CreateAccountFixture(ctx context.Context, t testing.TB, managerNodeID, label string, keys []string) string {
	if managerNodeID == "" {
		managerNodeID = CreateManagerNodeFixture(ctx, t, "", "", nil, nil)
	}
	if label == "" {
		label = fmt.Sprintf("acct-%d", <-accountCounter)
	}
	account, err := appdb.CreateAccount(ctx, managerNodeID, label, keys)
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return account.ID
}

var assetCounter = createCounter()

func CreateAssetFixture(ctx context.Context, t testing.TB, issuerNodeID, label, def string) bc.AssetID {
	if issuerNodeID == "" {
		issuerNodeID = CreateIssuerNodeFixture(ctx, t, "", "", nil, nil)
	}
	if label == "" {
		label = fmt.Sprintf("inode-%d", <-assetCounter)
	}
	asset, err := asset.Create(ctx, issuerNodeID, label, map[string]interface{}{"s": def})
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return asset.Hash
}

// Creates an infinite stream of integers counting up from 1
func createCounter() <-chan int {
	result := make(chan int)
	go func() {
		var n int
		for true {
			n++
			result <- n
		}
	}()
	return result
}

func NewContextWithGenesisBlock(tb testing.TB) context.Context {
	ctx := pgtest.NewContext(tb)

	key, err := testutil.TestXPrv.ECPrivKey()
	if err != nil {
		tb.Fatal(err)
	}
	asset.BlockKey = key

	_, err = asset.UpsertGenesisBlock(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	return ctx
}

func IssueAssetsFixture(ctx context.Context, t testing.TB, assetID bc.AssetID, amount uint64, accountID string) state.Output {
	if accountID == "" {
		accountID = CreateAccountFixture(ctx, t, "", "foo", nil)
	}
	dest := AccountDestinationFixture(ctx, t, assetID, amount, accountID)

	tpl, err := asset.Issue(ctx, assetID.String(), []*txbuilder.Destination{dest})
	if err != nil {
		testutil.FatalErr(t, err)
	}

	SignTxTemplate(t, tpl, testutil.TestXPrv)

	tx, err := asset.FinalizeTx(ctx, tpl)
	if err != nil {
		testutil.FatalErr(t, err)
	}

	return state.Output{
		Outpoint: bc.Outpoint{Hash: tx.Hash, Index: 0},
		TxOutput: *tx.Outputs[0],
	}
}

func AccountDestinationFixture(ctx context.Context, t testing.TB, assetID bc.AssetID, amount uint64, accountID string) *txbuilder.Destination {
	dest, err := asset.NewAccountDestination(ctx, &bc.AssetAmount{AssetID: assetID, Amount: amount}, accountID, nil)
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return dest
}

func IssuerTxFixture(ctx context.Context, t testing.TB, txHash string, data []byte, iNodeID string, asset string) (id string) {
	id, err := appdb.WriteIssuerTx(ctx, txHash, data, iNodeID, asset)
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return id
}

func ManagerTxFixture(ctx context.Context, t testing.TB, txHash string, data []byte, mNodeID string, accounts []string) (id string) {
	id, err := appdb.WriteManagerTx(ctx, txHash, data, mNodeID, accounts)
	if err != nil {
		testutil.FatalErr(t, err)
	}
	return id
}
