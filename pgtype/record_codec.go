package pgtype

import (
	"database/sql/driver"
	"fmt"
)

// ArrayGetter is a type that can be converted into a PostgreSQL array.

// RecordCodec is a codec for the generic PostgreSQL record type such as is created with the "row" function. Record can
// only decode the binary format. The text format output format from PostgreSQL does not include type information and
// is therefore impossible to decode. Encoding is impossible because PostgreSQL does not support input of generic
// records.
type RecordCodec struct{}

func (RecordCodec) FormatSupported(format int16) bool {
	return format == BinaryFormatCode
}

func (RecordCodec) PreferredFormat() int16 {
	return BinaryFormatCode
}

func (RecordCodec) PlanEncode(ci *ConnInfo, oid uint32, format int16, value interface{}) EncodePlan {
	return nil
}

func (RecordCodec) PlanScan(ci *ConnInfo, oid uint32, format int16, target interface{}, actualTarget bool) ScanPlan {
	if format == BinaryFormatCode {
		switch target.(type) {
		case CompositeIndexScanner:
			return &scanPlanBinaryRecordToCompositeIndexScanner{ci: ci}
		}
	}

	return nil
}

type scanPlanBinaryRecordToCompositeIndexScanner struct {
	ci *ConnInfo
}

func (plan *scanPlanBinaryRecordToCompositeIndexScanner) Scan(src []byte, target interface{}) error {
	targetScanner := (target).(CompositeIndexScanner)

	if src == nil {
		return targetScanner.ScanNull()
	}

	scanner := NewCompositeBinaryScanner(plan.ci, src)
	for i := 0; scanner.Next(); i++ {
		fieldTarget := targetScanner.ScanIndex(i)
		if fieldTarget != nil {
			fieldPlan := plan.ci.PlanScan(scanner.OID(), BinaryFormatCode, fieldTarget)
			if fieldPlan == nil {
				return fmt.Errorf("unable to scan OID %d in binary format into %v", scanner.OID(), fieldTarget)
			}

			err := fieldPlan.Scan(scanner.Bytes(), fieldTarget)
			if err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func (RecordCodec) DecodeDatabaseSQLValue(ci *ConnInfo, oid uint32, format int16, src []byte) (driver.Value, error) {
	if src == nil {
		return nil, nil
	}

	return nil, fmt.Errorf("not implemented")
}

func (RecordCodec) DecodeValue(ci *ConnInfo, oid uint32, format int16, src []byte) (interface{}, error) {
	if src == nil {
		return nil, nil
	}

	switch format {
	case TextFormatCode:
		return string(src), nil
	case BinaryFormatCode:
		scanner := NewCompositeBinaryScanner(ci, src)
		values := make([]interface{}, scanner.FieldCount())
		for i := 0; scanner.Next(); i++ {
			var v interface{}
			fieldPlan := ci.PlanScan(scanner.OID(), BinaryFormatCode, &v)
			if fieldPlan == nil {
				return nil, fmt.Errorf("unable to scan OID %d in binary format into %v", scanner.OID(), v)
			}

			err := fieldPlan.Scan(scanner.Bytes(), &v)
			if err != nil {
				return nil, err
			}

			values[i] = v
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}

		return values, nil
	default:
		return nil, fmt.Errorf("unknown format code %d", format)
	}

}
