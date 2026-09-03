package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"money/api/internal/models"
	"money/api/internal/store"
	"money/api/internal/testutil"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	return store.New(testutil.NewPool(t))
}

func TestAccountStore_CreateGetListUpdateDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	code := "1000"
	created, err := s.Accounts.Create(ctx, models.Account{Name: "Cash", Code: &code})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("Create: expected a non-zero ID")
	}

	got, err := s.Accounts.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Cash" || got.Code == nil || *got.Code != "1000" {
		t.Fatalf("Get: got %+v, want name=Cash code=1000", got)
	}

	second, err := s.Accounts.Create(ctx, models.Account{Name: "Bank"})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	list, err := s.Accounts.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID != created.ID || list[1].ID != second.ID {
		t.Fatalf("List: got %+v, want [%d, %d] in order", list, created.ID, second.ID)
	}

	got.Name = "Petty Cash"
	updated, err := s.Accounts.Update(ctx, got)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "Petty Cash" {
		t.Fatalf("Update: got name %q, want %q", updated.Name, "Petty Cash")
	}
	if again, err := s.Accounts.Get(ctx, created.ID); err != nil || again.Name != "Petty Cash" {
		t.Fatalf("Get after Update: got (%+v, %v)", again, err)
	}

	if err := s.Accounts.Delete(ctx, second.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Accounts.Get(ctx, second.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrNotFound", err)
	}
}

func TestAccountStore_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	const missingID = 999999

	if _, err := s.Accounts.Get(ctx, missingID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get: got %v, want ErrNotFound", err)
	}
	if _, err := s.Accounts.Update(ctx, models.Account{ID: missingID, Name: "X"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Update: got %v, want ErrNotFound", err)
	}
	if err := s.Accounts.Delete(ctx, missingID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Delete: got %v, want ErrNotFound", err)
	}
}

func TestAccountStore_ParentHierarchyAndRestrictedDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	parent, err := s.Accounts.Create(ctx, models.Account{Name: "Assets"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := s.Accounts.Create(ctx, models.Account{Name: "Cash", ParentID: &parent.ID})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	got, err := s.Accounts.Get(ctx, child.ID)
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != parent.ID {
		t.Fatalf("child ParentID = %v, want %d", got.ParentID, parent.ID)
	}

	err = s.Accounts.Delete(ctx, parent.ID)
	if !isPgErrorCode(err, "23503") { // foreign_key_violation
		t.Fatalf("deleting a parent with children: got %v, want foreign_key_violation", err)
	}
}

func TestAccountStore_DuplicateCodeRejected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	code := "1000"
	if _, err := s.Accounts.Create(ctx, models.Account{Name: "Cash", Code: &code}); err != nil {
		t.Fatalf("create first account: %v", err)
	}
	_, err := s.Accounts.Create(ctx, models.Account{Name: "Cash 2", Code: &code})
	if !isPgErrorCode(err, "23505") { // unique_violation
		t.Fatalf("creating a duplicate code: got %v, want unique_violation", err)
	}
}

func TestAccountStore_MoveSwapsWithSibling(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	a, err := s.Accounts.Create(ctx, models.Account{Name: "Assets"})
	if err != nil {
		t.Fatalf("create Assets: %v", err)
	}
	b, err := s.Accounts.Create(ctx, models.Account{Name: "Liabilities"})
	if err != nil {
		t.Fatalf("create Liabilities: %v", err)
	}
	if a.Position != 0 || b.Position != 1 {
		t.Fatalf("initial positions = %d, %d, want 0, 1", a.Position, b.Position)
	}

	if err := s.Accounts.Move(ctx, b.ID, store.MoveUp); err != nil {
		t.Fatalf("Move up: %v", err)
	}
	gotA, err := s.Accounts.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("get Assets: %v", err)
	}
	gotB, err := s.Accounts.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("get Liabilities: %v", err)
	}
	if gotA.Position != 1 || gotB.Position != 0 {
		t.Fatalf("positions after swap = %d, %d, want 1, 0", gotA.Position, gotB.Position)
	}
}

func TestAccountStore_MoveAtBoundaryIsNoop(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	a, err := s.Accounts.Create(ctx, models.Account{Name: "Assets"})
	if err != nil {
		t.Fatalf("create Assets: %v", err)
	}
	if _, err := s.Accounts.Create(ctx, models.Account{Name: "Liabilities"}); err != nil {
		t.Fatalf("create Liabilities: %v", err)
	}

	if err := s.Accounts.Move(ctx, a.ID, store.MoveUp); err != nil {
		t.Fatalf("Move up at boundary: %v", err)
	}
	got, err := s.Accounts.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("get Assets: %v", err)
	}
	if got.Position != 0 {
		t.Fatalf("Assets.Position after no-op move = %d, want unchanged 0", got.Position)
	}
}

func TestAccountStore_MoveOnlyAffectsSiblingsWithSameParent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	root, err := s.Accounts.Create(ctx, models.Account{Name: "Assets"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	child1, err := s.Accounts.Create(ctx, models.Account{Name: "Cash", ParentID: &root.ID})
	if err != nil {
		t.Fatalf("create child1: %v", err)
	}
	child2, err := s.Accounts.Create(ctx, models.Account{Name: "Bank", ParentID: &root.ID})
	if err != nil {
		t.Fatalf("create child2: %v", err)
	}

	// The two children are each other's only siblings; root (a different
	// parent group) must be unaffected by moving between them.
	if err := s.Accounts.Move(ctx, child2.ID, store.MoveUp); err != nil {
		t.Fatalf("Move up: %v", err)
	}
	gotRoot, err := s.Accounts.Get(ctx, root.ID)
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	if gotRoot.Position != 0 {
		t.Fatalf("root.Position = %d, want unchanged 0", gotRoot.Position)
	}
	gotChild1, err := s.Accounts.Get(ctx, child1.ID)
	if err != nil {
		t.Fatalf("get child1: %v", err)
	}
	gotChild2, err := s.Accounts.Get(ctx, child2.ID)
	if err != nil {
		t.Fatalf("get child2: %v", err)
	}
	if gotChild1.Position != 1 || gotChild2.Position != 0 {
		t.Fatalf("children positions after swap = %d, %d, want 1, 0", gotChild1.Position, gotChild2.Position)
	}
}

func TestAccountStore_MoveNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if err := s.Accounts.Move(ctx, 999999, store.MoveUp); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Move on missing account: got %v, want ErrNotFound", err)
	}
}

func TestAccountStore_UpdateRejectsSelfAsParent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	a, err := s.Accounts.Create(ctx, models.Account{Name: "Assets"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = s.Accounts.Update(ctx, models.Account{ID: a.ID, Name: "Assets", ParentID: &a.ID})
	if !errors.Is(err, store.ErrCycle) {
		t.Fatalf("update with itself as parent: got %v, want ErrCycle", err)
	}

	got, err := s.Accounts.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ParentID != nil {
		t.Fatalf("ParentID after rejected update = %v, want unchanged nil", got.ParentID)
	}
}

func TestAccountStore_UpdateRejectsDescendantAsParent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	grandparent, err := s.Accounts.Create(ctx, models.Account{Name: "Assets"})
	if err != nil {
		t.Fatalf("create grandparent: %v", err)
	}
	parent, err := s.Accounts.Create(ctx, models.Account{Name: "Banks", ParentID: &grandparent.ID})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := s.Accounts.Create(ctx, models.Account{Name: "Checking", ParentID: &parent.ID})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	// Directly: grandparent's parent can't be its immediate child.
	_, err = s.Accounts.Update(ctx, models.Account{ID: grandparent.ID, Name: "Assets", ParentID: &parent.ID})
	if !errors.Is(err, store.ErrCycle) {
		t.Fatalf("update with a child as parent: got %v, want ErrCycle", err)
	}

	// Transitively: grandparent's parent can't be its grandchild either.
	_, err = s.Accounts.Update(ctx, models.Account{ID: grandparent.ID, Name: "Assets", ParentID: &child.ID})
	if !errors.Is(err, store.ErrCycle) {
		t.Fatalf("update with a grandchild as parent: got %v, want ErrCycle", err)
	}
}

func TestAccountStore_UpdateAllowsReparentingToUnrelatedAccount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	assets, err := s.Accounts.Create(ctx, models.Account{Name: "Assets"})
	if err != nil {
		t.Fatalf("create Assets: %v", err)
	}
	liabilities, err := s.Accounts.Create(ctx, models.Account{Name: "Liabilities"})
	if err != nil {
		t.Fatalf("create Liabilities: %v", err)
	}
	child, err := s.Accounts.Create(ctx, models.Account{Name: "Cash", ParentID: &assets.ID})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	updated, err := s.Accounts.Update(ctx, models.Account{ID: child.ID, Name: "Cash", ParentID: &liabilities.ID})
	if err != nil {
		t.Fatalf("reparent to an unrelated account: %v", err)
	}
	if updated.ParentID == nil || *updated.ParentID != liabilities.ID {
		t.Fatalf("ParentID after reparenting = %v, want %d", updated.ParentID, liabilities.ID)
	}
}

func isPgErrorCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
