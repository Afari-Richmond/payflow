package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
)

// testDB opens a connection to the real payment-service database used
// for local development (docker-compose). Skips the test if it's
// unreachable, so `go test` doesn't hard-fail on a machine without
// Postgres running.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbURL := os.Getenv("PAYMENT_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://payflow:payflow@localhost:5433/payflow_payment?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("skipping: payment-service database unreachable: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil || sqlDB.Ping() != nil {
		t.Skip("skipping: payment-service database unreachable")
	}

	return db
}

func TestGormPaymentRepository_CreateAndGetByID(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormPaymentRepository(db)
	ctx := context.Background()

	payment := &domain.Payment{
		ID:          uuid.New(),
		OrderID:     uuid.New(),
		AmountMinor: 25000,
		Currency:    "GHS",
		Status:      domain.StatusPending,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM payments WHERE id = ?", payment.ID)
	})

	if err := repo.Create(ctx, payment); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(ctx, payment.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if got.OrderID != payment.OrderID {
		t.Errorf("expected order id %v, got %v", payment.OrderID, got.OrderID)
	}
	if got.AmountMinor != payment.AmountMinor {
		t.Errorf("expected amount %d, got %d", payment.AmountMinor, got.AmountMinor)
	}
	if got.Currency != payment.Currency {
		t.Errorf("expected currency %q, got %q", payment.Currency, got.Currency)
	}
	if got.Status != payment.Status {
		t.Errorf("expected status %q, got %q", payment.Status, got.Status)
	}
}

func TestGormPaymentRepository_GetByID_NotFound(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormPaymentRepository(db)

	_, err := repo.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
