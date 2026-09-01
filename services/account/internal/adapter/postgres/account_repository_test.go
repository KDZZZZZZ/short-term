package postgres_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pgtest"
	"github.com/KDZZZZZZ/short-term/services/account/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/account/internal/application"
	"github.com/KDZZZZZZ/short-term/services/account/internal/domain"
	"github.com/KDZZZZZZ/short-term/services/account/migrations"
)

func newRepository(t *testing.T) (*postgres.AccountRepository, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.New(t, migrations.FS, migrations.Dir)
	return postgres.NewAccountRepository(pool), pool
}

func newAccount(t *testing.T, id, studentNo string) *domain.Account {
	t.Helper()

	wechat := "wx_" + studentNo
	account, err := domain.New(id, studentNo, "$argon2id$stub", "同学"+studentNo, &wechat, nil,
		time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	return account
}

func TestCreateAndRead(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)
	want := newAccount(t, "u_1", "20260001")

	if err := repo.Create(t.Context(), want); err != nil {
		t.Fatalf("Create: %v", err)
	}

	byID, err := repo.ByID(t.Context(), "u_1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if byID.StudentNo != want.StudentNo || byID.Nickname != want.Nickname {
		t.Fatalf("ByID = %+v, want %+v", byID, want)
	}
	if byID.Wechat == nil || *byID.Wechat != *want.Wechat {
		t.Fatalf("Wechat = %v, want %v", byID.Wechat, *want.Wechat)
	}
	if byID.QQ != nil {
		t.Fatalf("QQ = %v, want nil", *byID.QQ)
	}
	if !byID.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("CreatedAt = %s, want %s", byID.CreatedAt, want.CreatedAt)
	}

	byStudentNo, err := repo.ByStudentNo(t.Context(), "20260001")
	if err != nil {
		t.Fatalf("ByStudentNo: %v", err)
	}
	if byStudentNo.ID != "u_1" {
		t.Fatalf("ByStudentNo returned %q", byStudentNo.ID)
	}
}

func TestReadMissingAccount(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)

	if _, err := repo.ByID(t.Context(), "u_missing"); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("ByID = %v, want ErrNotFound", err)
	}
	if _, err := repo.ByStudentNo(t.Context(), "20269999"); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("ByStudentNo = %v, want ErrNotFound", err)
	}
}

func TestCreateRejectsDuplicateStudentNumber(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)
	if err := repo.Create(t.Context(), newAccount(t, "u_1", "20260001")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	err := repo.Create(t.Context(), newAccount(t, "u_2", "20260001"))
	if !errors.Is(err, application.ErrStudentNoTaken) {
		t.Fatalf("Create = %v, want ErrStudentNoTaken", err)
	}
}

func TestConcurrentRegistrationsOfOneStudentNumber(t *testing.T) {
	t.Parallel()

	repo, pool := newRepository(t)

	const attempts = 8
	results := make([]error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = repo.Create(t.Context(), newAccount(t, "u_"+string(rune('a'+i)), "20260001"))
		}()
	}
	wg.Wait()

	var created int
	for _, err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, application.ErrStudentNoTaken):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if created != 1 {
		t.Fatalf("%d concurrent registrations succeeded, want exactly 1", created)
	}

	var stored int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM accounts WHERE student_no = $1`, "20260001").Scan(&stored); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if stored != 1 {
		t.Fatalf("stored rows = %d, want 1", stored)
	}
}

func TestByIDsReturnsOnlyExistingAccounts(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)
	for i, studentNo := range []string{"20260001", "20260002", "20260003"} {
		if err := repo.Create(t.Context(), newAccount(t, "u_"+string(rune('1'+i)), studentNo)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	accounts, err := repo.ByIDs(t.Context(), []string{"u_1", "u_3", "u_missing"})
	if err != nil {
		t.Fatalf("ByIDs: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("got %d accounts, want 2", len(accounts))
	}

	found := map[string]bool{}
	for _, account := range accounts {
		found[account.ID] = true
	}
	if !found["u_1"] || !found["u_3"] {
		t.Fatalf("ByIDs returned %v", found)
	}
}

func TestUpdateWritesBackMutableFields(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)
	account := newAccount(t, "u_1", "20260001")
	if err := repo.Create(t.Context(), account); err != nil {
		t.Fatalf("Create: %v", err)
	}

	later := account.CreatedAt.Add(time.Hour)
	if err := account.Rename("新昵称", later); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := account.SetWechat(nil, later); err != nil {
		t.Fatalf("SetWechat: %v", err)
	}
	qq := "123456789"
	if err := account.SetQQ(&qq, later); err != nil {
		t.Fatalf("SetQQ: %v", err)
	}
	if err := repo.Update(t.Context(), account); err != nil {
		t.Fatalf("Update: %v", err)
	}

	reloaded, err := repo.ByID(t.Context(), "u_1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if reloaded.Nickname != "新昵称" {
		t.Fatalf("Nickname = %q", reloaded.Nickname)
	}
	if reloaded.Wechat != nil {
		t.Fatalf("Wechat = %v, want nil after clearing", *reloaded.Wechat)
	}
	if reloaded.QQ == nil || *reloaded.QQ != qq {
		t.Fatalf("QQ = %v, want %s", reloaded.QQ, qq)
	}
	if reloaded.StudentNo != "20260001" {
		t.Fatalf("Update changed the student number to %q", reloaded.StudentNo)
	}
}

func TestUpdateMissingAccount(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)

	err := repo.Update(t.Context(), newAccount(t, "u_missing", "20260001"))
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("Update = %v, want ErrNotFound", err)
	}
}

func TestDatabaseRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	_, pool := newRepository(t)

	// The migration's CHECK constraints are a second line of defence behind
	// domain validation; a repository bug must not be able to store a value the
	// public contract cannot represent.
	tests := []struct {
		name      string
		studentNo string
		nickname  string
		qq        any
	}{
		{name: "student number too short", studentNo: "1", nickname: "n", qq: nil},
		{name: "student number with a space", studentNo: "2026 0001", nickname: "n", qq: nil},
		{name: "empty nickname", studentNo: "20260001", nickname: "", qq: nil},
		{name: "qq with letters", studentNo: "20260001", nickname: "n", qq: "12345abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := pool.Exec(t.Context(),
				`INSERT INTO accounts (id, student_no, password_hash, nickname, qq) VALUES ($1, $2, $3, $4, $5)`,
				"u_"+tt.name, tt.studentNo, "hash", tt.nickname, tt.qq)
			if err == nil {
				t.Fatal("the database accepted a value the contract forbids")
			}
		})
	}
}
