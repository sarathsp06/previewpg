package sqlparser

import (
	"github.com/xwb1989/sqlparser"
)

type StmtType int

const (
	Read StmtType = iota
	Write
	DDL
	Transaction
)

func GetStmtType(sql string) (StmtType, error) {
	stmt, err := sqlparser.Parse(sql)
	if err != nil {
		return Read, err // Default to read for safety
	}

	switch stmt.(type) {
	case *sqlparser.Select, *sqlparser.Show:
		return Read, nil
	case *sqlparser.Insert, *sqlparser.Update, *sqlparser.Delete:
		return Write, nil
	case *sqlparser.DDL:
		return DDL, nil
	case *sqlparser.Set, *sqlparser.Begin, *sqlparser.Commit, *sqlparser.Rollback:
		return Transaction, nil
	default:
		return Read, nil // Default to read for safety
	}
}
