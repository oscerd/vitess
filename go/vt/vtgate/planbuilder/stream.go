/*
Copyright 2021 The Vitess Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package planbuilder

import (
	"fmt"
	"strconv"
	"strings"

	"vitess.io/vitess/go/vt/key"
	topodatapb "vitess.io/vitess/go/vt/proto/topodata"
	vtrpcpb "vitess.io/vitess/go/vt/proto/vtrpc"
	"vitess.io/vitess/go/vt/sqlparser"
	"vitess.io/vitess/go/vt/vterrors"
	"vitess.io/vitess/go/vt/vtgate/engine"
)

const defaultLimit = 100

func buildStreamPlan(stmt *sqlparser.Stream, vschema ContextVSchema) (engine.Primitive, error) {
	table, _, destTabletType, dest, err := vschema.FindTable(stmt.Table)
	if err != nil {
		return nil, err
	}
	if destTabletType != topodatapb.TabletType_PRIMARY {
		return nil, vterrors.Errorf(vtrpcpb.Code_FAILED_PRECONDITION, "stream is supported only for primary tablet type, current type: %v", destTabletType)
	}
	if dest == nil {
		dest = key.DestinationExactKeyRange{}
	}
	return &engine.MStream{
		Keyspace:          table.Keyspace,
		TargetDestination: dest,
		TableName:         table.Name.CompliantName(),
	}, nil
}

func buildVStreamPlan(stmt *sqlparser.VStream, vschema ContextVSchema) (engine.Primitive, error) {
	table, _, destTabletType, dest, err := vschema.FindTable(stmt.Table)
	if err != nil {
		return nil, err
	}
	if destTabletType != topodatapb.TabletType_PRIMARY {
		return nil, vterrors.Errorf(vtrpcpb.Code_FAILED_PRECONDITION, "vstream is supported only for primary tablet type, current type: %v", destTabletType)
	}
	if dest == nil {
		dest = key.DestinationAllShards{}
	}
	var pos string
	if stmt.Where != nil {
		pos, err = getVStreamStartPos(stmt)
		if err != nil {
			return nil, err
		}
	}
	limit := defaultLimit
	if stmt.Limit != nil {
		count, ok := stmt.Limit.Rowcount.(*sqlparser.Literal)
		if ok {
			limit, _ = strconv.Atoi(count.Val)
		}
	}

	return &engine.VStream{
		Keyspace:          table.Keyspace,
		TargetDestination: dest,
		TableName:         table.Name.CompliantName(),
		Position:          pos,
		Limit:             limit,
	}, nil
}

func getVStreamStartPos(stmt *sqlparser.VStream) (string, error) {
	var colName, pos string
	if stmt.Where != nil {
		switch v := stmt.Where.Expr.(type) {
		case *sqlparser.ComparisonExpr:
			if v.Operator == sqlparser.GreaterThanOp {
				switch c := v.Left.(type) {
				case *sqlparser.ColName:
					switch val := v.Right.(type) {
					case *sqlparser.Literal:
						pos = string(val.Val)
					}
					colName = strings.ToLower(c.Name.String())
					if colName != "pos" {
						return "", vterrors.NewErrorf(vtrpcpb.Code_INVALID_ARGUMENT, vterrors.SyntaxError, "can only use pos in vstream where clause ")
					}
				}
			} else {
				return "", fmt.Errorf("where can only be of type 'pos > <value>'")
			}
		default:
			return "", fmt.Errorf("where can only be of type 'pos > <value>'")
		}
	}
	return pos, nil
}
