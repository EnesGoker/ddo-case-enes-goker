package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL hata kodları
const (
	codeUniqueViolation    = "23505"
	codeExclusionViolation = "23P01"
)

// Violates err'in adı verilen kısıtlardan birinin benzersizlik (23505) veya dışlama (23P01)
// ihlali olup olmadığını söyler. Servisler bunu iş hatasına çevirir: asıl koruma kilit ve
// durum kontrolüdür, kısıt ikinci savunma hattıdır ve 500 yerine anlamlı bir yanıt üretmelidir.
func Violates(err error, constraints ...string) bool {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || (pg.Code != codeUniqueViolation && pg.Code != codeExclusionViolation) {
		return false
	}
	for _, c := range constraints {
		if pg.ConstraintName == c {
			return true
		}
	}
	return false
}
