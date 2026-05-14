package mysqlstore

import (
	"errors"

	"github.com/go-sql-driver/mysql"
)

// IsFKViolation reports MySQL errno 1452 (cannot add or update a child row).
func IsFKViolation(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1452
}
