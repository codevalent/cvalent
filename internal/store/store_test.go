package store

import (
	"context"
	"testing"

	"github.com/codevalent/cvalent/internal/model"
)

func openMem(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mintFn(t *testing.T, name string) model.FunctionNode {
	t.Helper()
	parts := model.IdentityParts{
		Distribution: "github.com/foo/bar",
		ModulePath:   "internal/widget",
		Name:         name,
	}
	id, canon, err := model.MintFunctionID(model.EnvironmentLocal, parts)
	if err != nil {
		t.Fatal(err)
	}
	return model.FunctionNode{
		Node: model.Node{
			ID:             id,
			Environment:    model.EnvironmentLocal,
			Kind:           model.KindFunction,
			QualifiedName:  canon.QualifiedName(),
			Name:           name,
			Distribution:   "github.com/foo/bar",
			ModulePath:     "internal/widget",
			Language:       "go",
			File:           "internal/widget/x.go",
			IdentitySource: model.IdentityFromDistribution,
		},
		FunctionMeta: model.FunctionMeta{
			StartLine:            10,
			EndLine:              20,
			Exported:             true,
			Tag:                  "application",
			ContractCompleteness: "full",
			Params: []model.Param{
				{Name: "x", Type: "int"},
				{Name: "s", Type: "string"},
			},
			Returns: model.ReturnSpec{
				Values: []model.ReturnValue{{Type: "error", Nullable: true}},
			},
		},
	}
}

func TestStore_OpenRunsMigrations(t *testing.T) {
	s := openMem(t)
	// goose tables exist? At minimum, our nodes table must exist.
	if _, err := s.db.Exec(`SELECT 1 FROM nodes LIMIT 1`); err != nil {
		t.Fatalf("nodes table not created: %v", err)
	}
}

func TestStore_RoundTripFunctionNode(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	fn := mintFn(t, "Frob")
	if err := s.UpsertNode(ctx, fn); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	got, err := s.GetNode(ctx, fn.ID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.ID != fn.ID || got.QualifiedName != fn.QualifiedName || got.Name != fn.Name {
		t.Fatalf("identity mismatch: %+v vs %+v", got, fn)
	}
	if len(got.Params) != 2 || got.Params[0].Name != "x" || got.Params[1].Type != "string" {
		t.Fatalf("params not round-tripped: %+v", got.Params)
	}
	if len(got.Returns.Values) != 1 || got.Returns.Values[0].Type != "error" {
		t.Fatalf("returns not round-tripped: %+v", got.Returns)
	}
}

func TestStore_BitemporalDefaultsAtEpoch(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	fn := mintFn(t, "Frob")
	if err := s.UpsertNode(ctx, fn); err != nil {
		t.Fatal(err)
	}
	row := s.db.QueryRow(`SELECT valid_from, valid_until FROM nodes WHERE valid_until IS NULL`)
	var validFrom string
	var validUntil any
	if err := row.Scan(&validFrom, &validUntil); err != nil {
		t.Fatal(err)
	}
	if validFrom != Epoch {
		t.Errorf("valid_from = %q want %q", validFrom, Epoch)
	}
	if validUntil != nil {
		t.Errorf("valid_until = %v want nil", validUntil)
	}
}

func TestStore_ListNodesByKind(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	for _, name := range []string{"Alpha", "Bravo", "Charlie"} {
		if err := s.UpsertNode(ctx, mintFn(t, name)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListNodesByKind(ctx, model.KindFunction)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("want 3 nodes, got %d", len(got))
	}
}

func TestStore_UpsertIsIdempotent(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	fn := mintFn(t, "Frob")
	if err := s.UpsertNode(ctx, fn); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertNode(ctx, fn); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&n)
	if n != 1 {
		t.Errorf("want 1 row, got %d", n)
	}
}
