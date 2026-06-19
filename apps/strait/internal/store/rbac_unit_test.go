package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func projectRoleScanFn(now time.Time, withParent bool, isSystem bool) func(dest ...any) error {
	return func(dest ...any) error {
		description := "role description"
		*(dest[0].(*string)) = "role-1"
		*(dest[1].(*string)) = "project-1"
		*(dest[2].(*string)) = "operator"
		*(dest[3].(**string)) = &description
		*(dest[4].(*[]string)) = []string{"jobs:read", "runs:read"}
		if withParent {
			parent := "parent-role"
			*(dest[5].(**string)) = &parent
		}
		*(dest[6].(*bool)) = isSystem
		*(dest[7].(*time.Time)) = now
		*(dest[8].(*time.Time)) = now.Add(time.Second)
		return nil
	}
}

type rbacTx struct {
	*fakeTx
	commits   int
	rollbacks int
}

func (tx *rbacTx) Commit(context.Context) error {
	tx.commits++
	return nil
}

func (tx *rbacTx) Rollback(context.Context) error {
	tx.rollbacks++
	return nil
}

type rbacBeginner struct {
	mockDBTX
	tx *rbacTx
}

func (b *rbacBeginner) Begin(context.Context) (pgx.Tx, error) {
	return b.tx, nil
}

func TestRBACProjectRolesUnit(t *testing.T) {
	t.Parallel()

	t.Run("creates project role with defaults and parent validation", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var queryRows int
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				queryRows++
				switch {
				case strings.Contains(sql, "SELECT EXISTS"):
					require.Equal(t, []any{"parent-role", "project-1"}, args)
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*bool)) = true
						return nil
					}}
				case strings.Contains(sql, "INSERT INTO project_roles"):
					require.NotEmpty(t, args[0])
					require.Equal(t, "project-1", args[1])
					require.Equal(t, "parent-role", args[5])
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*time.Time)) = now
						*(dest[1].(*time.Time)) = now
						return nil
					}}
				default:
					require.Failf(t, "unexpected query", "sql=%s args=%v", sql, args)
					return &mockRow{}
				}
			},
		}
		role := &domain.ProjectRole{ProjectID: "project-1", Name: "operator", Permissions: []string{"jobs:read"}, ParentRoleID: "parent-role"}

		require.NoError(t, New(db).CreateProjectRole(context.Background(), role))
		require.NotEmpty(t, role.ID)
		require.Equal(t, 2, queryRows)

		err := New(&mockDBTX{}).CreateProjectRole(context.Background(), &domain.ProjectRole{ID: "role-1", ParentRoleID: "role-1"})
		require.ErrorContains(t, err, "parent role cannot reference itself")

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = false
				return nil
			}}
		}
		err = New(db).CreateProjectRole(context.Background(), &domain.ProjectRole{ProjectID: "project-1", ParentRoleID: "foreign-role"})
		require.ErrorIs(t, err, ErrRoleNotFound)

		createErr := errors.New("insert failed")
		db.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "SELECT EXISTS") {
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				}}
			}
			return &mockRow{scanFn: func(...any) error { return createErr }}
		}
		err = New(db).CreateProjectRole(context.Background(), &domain.ProjectRole{ProjectID: "project-1", ParentRoleID: "parent-role"})
		require.ErrorContains(t, err, "create project role")
		require.ErrorIs(t, err, createErr)
	})

	t.Run("gets lists updates and deletes roles", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				switch {
				case strings.Contains(sql, "SELECT id, project_id"):
					require.Equal(t, []any{"role-1"}, args)
					return &mockRow{scanFn: projectRoleScanFn(now, false, false)}
				case strings.Contains(sql, "UPDATE project_roles"):
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*time.Time)) = now.Add(2 * time.Second)
						return nil
					}}
				default:
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*bool)) = false
						return nil
					}}
				}
			},
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "created_at < $2")
				require.Equal(t, []any{"project-1", now, 10}, args)
				return &mockRows{scanFns: []func(dest ...any) error{projectRoleScanFn(now, true, false)}}, nil
			},
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM project_roles")
				require.Equal(t, []any{"role-1"}, args)
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}

		role, err := New(db).GetProjectRole(context.Background(), "role-1")
		require.NoError(t, err)
		require.Equal(t, "role description", role.Description)

		roles, err := New(db).ListProjectRoles(context.Background(), "project-1", 10, &now)
		require.NoError(t, err)
		require.Len(t, roles, 1)
		require.Equal(t, "parent-role", roles[0].ParentRoleID)

		require.NoError(t, New(db).UpdateProjectRole(context.Background(), &domain.ProjectRole{ID: "role-1", Name: "updated"}))
		require.NoError(t, New(db).DeleteProjectRole(context.Background(), "role-1"))

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		_, err = New(db).GetProjectRole(context.Background(), "missing")
		require.ErrorIs(t, err, ErrRoleNotFound)
		require.ErrorIs(t, New(db).UpdateProjectRole(context.Background(), &domain.ProjectRole{ID: "missing"}), ErrRoleNotFound)

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		}
		require.ErrorIs(t, New(db).DeleteProjectRole(context.Background(), "missing"), ErrRoleNotFound)
	})

	t.Run("update blocks system roles self parents and cycles", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tests := []struct {
			name       string
			role       *domain.ProjectRole
			existing   func(dest ...any) error
			cycle      bool
			wantString string
			wantIs     error
		}{
			{name: "system", role: &domain.ProjectRole{ID: "role-1"}, existing: projectRoleScanFn(now, false, true), wantIs: ErrRoleNotFound},
			{name: "self parent", role: &domain.ProjectRole{ID: "role-1", ParentRoleID: "role-1"}, existing: projectRoleScanFn(now, false, false), wantString: "parent role cannot reference itself"},
			{name: "cycle", role: &domain.ProjectRole{ID: "role-1", ParentRoleID: "parent-role"}, existing: projectRoleScanFn(now, false, false), cycle: true, wantString: "parent role would create a cycle"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var rowCalls int
				db := &mockDBTX{
					queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
						rowCalls++
						switch {
						case rowCalls == 1:
							return &mockRow{scanFn: tc.existing}
						case strings.Contains(sql, "SELECT EXISTS") && strings.Contains(sql, "WHERE id = $1 AND project_id"):
							return &mockRow{scanFn: func(dest ...any) error {
								*(dest[0].(*bool)) = true
								return nil
							}}
						default:
							return &mockRow{scanFn: func(dest ...any) error {
								*(dest[0].(*bool)) = tc.cycle
								return nil
							}}
						}
					},
				}

				err := New(db).UpdateProjectRole(context.Background(), tc.role)
				if tc.wantIs != nil {
					require.ErrorIs(t, err, tc.wantIs)
				} else {
					require.ErrorContains(t, err, tc.wantString)
				}
			})
		}
	})
}

func TestRBACMembersAndPermissionsUnit(t *testing.T) {
	t.Parallel()

	t.Run("assign get remove and list member roles", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		grantedBy := "admin"
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				switch {
				case strings.Contains(sql, "INSERT INTO project_member_roles"):
					require.NotEmpty(t, args[0])
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*time.Time)) = now
						return nil
					}}
				default:
					require.Equal(t, []any{"project-1", "user-1"}, args)
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*string)) = "member-1"
						*(dest[1].(*string)) = "project-1"
						*(dest[2].(*string)) = "user-1"
						*(dest[3].(*string)) = "role-1"
						*(dest[4].(**string)) = &grantedBy
						*(dest[5].(*time.Time)) = now
						return nil
					}}
				}
			},
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "created_at < $2")
				require.Equal(t, []any{"project-1", now, 5}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "member-1"
						*(dest[1].(*string)) = "project-1"
						*(dest[2].(*string)) = "user-1"
						*(dest[3].(*string)) = "role-1"
						*(dest[4].(**string)) = &grantedBy
						*(dest[5].(*time.Time)) = now
						return nil
					},
				}}, nil
			},
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "DELETE FROM project_member_roles")
				require.Equal(t, []any{"project-1", "user-1"}, args)
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}

		member := &domain.ProjectMemberRole{ProjectID: "project-1", UserID: "user-1", RoleID: "role-1", GrantedBy: "admin"}
		require.NoError(t, New(db).AssignMemberRole(context.Background(), member))
		require.NotEmpty(t, member.ID)

		got, err := New(db).GetMemberRole(context.Background(), "project-1", "user-1")
		require.NoError(t, err)
		require.Equal(t, "admin", got.GrantedBy)

		members, err := New(db).ListProjectMembers(context.Background(), "project-1", 5, &now)
		require.NoError(t, err)
		require.Len(t, members, 1)

		require.NoError(t, New(db).RemoveMemberRole(context.Background(), "project-1", "user-1"))

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		got, err = New(db).GetMemberRole(context.Background(), "project-1", "missing")
		require.NoError(t, err)
		require.Nil(t, got)

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		}
		require.ErrorIs(t, New(db).RemoveMemberRole(context.Background(), "project-1", "missing"), ErrMemberNotFound)
	})

	t.Run("assign member role with org limit", func(t *testing.T) {
		t.Parallel()

		require.ErrorContains(t, New(&mockDBTX{}).AssignMemberRoleWithOrgLimit(context.Background(), nil, "org-1", 1), "member is nil")

		now := time.Now().UTC()
		tests := []struct {
			name         string
			already      bool
			count        int
			wantLimitErr bool
		}{
			{name: "existing member bypasses count", already: true},
			{name: "new member under limit", count: 1},
			{name: "new member over limit", count: 2, wantLimitErr: true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var queryRows int
				tx := &rbacTx{}
				tx.fakeTx = &fakeTx{
					execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
						require.True(t, strings.Contains(sql, "pg_advisory_xact_lock") || strings.Contains(sql, "INSERT INTO project_member_roles"))
						return pgconn.NewCommandTag("OK"), nil
					},
					queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
						queryRows++
						switch {
						case strings.Contains(sql, "SELECT EXISTS"):
							return &mockRow{scanFn: func(dest ...any) error {
								*(dest[0].(*bool)) = tc.already
								return nil
							}}
						case strings.Contains(sql, "COUNT(DISTINCT"):
							return &mockRow{scanFn: func(dest ...any) error {
								*(dest[0].(*int)) = tc.count
								return nil
							}}
						default:
							return &mockRow{scanFn: func(dest ...any) error {
								*(dest[0].(*time.Time)) = now
								return nil
							}}
						}
					},
				}

				err := New(&rbacBeginner{tx: tx}).AssignMemberRoleWithOrgLimit(context.Background(), &domain.ProjectMemberRole{ProjectID: "project-1", UserID: "user-1", RoleID: "role-1"}, "org-1", 2)
				if tc.wantLimitErr {
					require.ErrorIs(t, err, ErrMemberLimitReached)
					require.Zero(t, tx.commits)
					require.Equal(t, 1, tx.rollbacks)
					return
				}
				require.NoError(t, err)
				require.Equal(t, 1, tx.commits)
				require.Positive(t, queryRows)
			})
		}
	})

	t.Run("deduplicates permissions and reports version", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "WITH RECURSIVE role_tree")
				require.Equal(t, []any{"project-1", "user-1"}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*[]string)) = []string{"jobs:read", "jobs:write"}
						*(dest[1].(*int64)) = 7
						return nil
					},
					func(dest ...any) error {
						*(dest[0].(*[]string)) = []string{"jobs:read", "runs:read"}
						*(dest[1].(*int64)) = 9
						return nil
					},
				}}, nil
			},
		}
		perms, version, err := New(db).GetUserPermissionsWithVersion(context.Background(), "project-1", "user-1")
		require.NoError(t, err)
		require.Equal(t, []string{"jobs:read", "jobs:write", "runs:read"}, perms)
		require.EqualValues(t, 9, version)

		accessDB := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = true
				return nil
			}}
		}}
		ok, err := New(accessDB).UserHasProjectAccess(context.Background(), "user-1", "project-1")
		require.NoError(t, err)
		require.True(t, ok)

		db.queryFn = func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{}, nil
		}
		perms, version, err = New(db).GetUserPermissionsWithVersion(context.Background(), "project-1", "missing")
		require.NoError(t, err)
		require.Nil(t, perms)
		require.Zero(t, version)
	})
}

func TestRBACPoliciesUnit(t *testing.T) {
	t.Parallel()

	t.Run("resource policies create get delete and list", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				switch {
				case strings.Contains(sql, "INSERT INTO resource_policies"):
					require.NotEmpty(t, args[0])
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*time.Time)) = now
						return nil
					}}
				case strings.Contains(sql, "SELECT actions"):
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*[]string)) = []string{"read"}
						return nil
					}}
				default:
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*string)) = "project-1"
						*(dest[1].(*string)) = "user-1"
						return nil
					}}
				}
			},
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "created_at < $4")
				require.Equal(t, []any{"project-1", "job", "job-1", now, 10}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "policy-1"
						*(dest[1].(*string)) = "project-1"
						*(dest[2].(*string)) = "job"
						*(dest[3].(*string)) = "job-1"
						*(dest[4].(*string)) = "user-1"
						*(dest[5].(*[]string)) = []string{"read"}
						*(dest[6].(*time.Time)) = now
						return nil
					},
				}}, nil
			},
		}
		policy := &domain.ResourcePolicy{ProjectID: "project-1", ResourceType: "job", ResourceID: "job-1", UserID: "user-1", Actions: []string{"read"}}
		require.NoError(t, New(db).CreateResourcePolicy(context.Background(), policy))
		require.NotEmpty(t, policy.ID)

		actions, err := New(db).GetResourcePolicies(context.Background(), "project-1", "job", "job-1", "user-1")
		require.NoError(t, err)
		require.Equal(t, []string{"read"}, actions)

		deletedProject, userID, err := New(db).DeleteResourcePolicy(context.Background(), "project-1", "policy-1")
		require.NoError(t, err)
		require.Equal(t, "project-1", deletedProject)
		require.Equal(t, "user-1", userID)

		list, err := New(db).ListResourcePolicies(context.Background(), "project-1", "job", "job-1", 10, &now)
		require.NoError(t, err)
		require.Len(t, list, 1)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		actions, err = New(db).GetResourcePolicies(context.Background(), "project-1", "job", "job-1", "missing")
		require.NoError(t, err)
		require.Nil(t, actions)
		_, _, err = New(db).DeleteResourcePolicy(context.Background(), "project-1", "missing")
		require.ErrorIs(t, err, ErrResourcePolicyNotFound)
	})

	t.Run("tag policies create list delete and match actions", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tagValue := "payments"
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				switch {
				case strings.Contains(sql, "INSERT INTO tag_policies"):
					require.Nil(t, args[5])
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*time.Time)) = now
						return nil
					}}
				default:
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*string)) = "project-1"
						*(dest[1].(*string)) = "user-1"
						return nil
					}}
				}
			},
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				if strings.Contains(sql, "SELECT tag_key, tag_value, actions") {
					require.Equal(t, []any{"project-1", "job", "user-1"}, args)
					return &mockRows{scanFns: []func(dest ...any) error{
						func(dest ...any) error {
							*(dest[0].(*string)) = "team"
							*(dest[1].(**string)) = &tagValue
							*(dest[2].(*[]string)) = []string{"read", "write"}
							return nil
						},
						func(dest ...any) error {
							*(dest[0].(*string)) = "team"
							*(dest[1].(**string)) = &tagValue
							*(dest[2].(*[]string)) = []string{"read"}
							return nil
						},
						func(dest ...any) error {
							*(dest[0].(*string)) = "region"
							*(dest[2].(*[]string)) = []string{"ignored"}
							return nil
						},
					}}, nil
				}
				require.Contains(t, sql, "resource_type = $2")
				require.Contains(t, sql, "user_id = $3")
				require.Contains(t, sql, "created_at < $4")
				require.Equal(t, []any{"project-1", "job", "user-1", now, 10}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "tag-policy-1"
						*(dest[1].(*string)) = "project-1"
						*(dest[2].(*string)) = "job"
						*(dest[3].(*string)) = "user-1"
						*(dest[4].(*string)) = "team"
						*(dest[5].(**string)) = &tagValue
						*(dest[6].(*[]string)) = []string{"read"}
						*(dest[7].(*time.Time)) = now
						return nil
					},
				}}, nil
			},
		}

		policy := &domain.TagPolicy{ProjectID: "project-1", ResourceType: "job", UserID: "user-1", TagKey: "team", Actions: []string{"read"}}
		require.NoError(t, New(db).CreateTagPolicy(context.Background(), policy))
		require.NotEmpty(t, policy.ID)

		list, err := New(db).ListTagPolicies(context.Background(), "project-1", "job", "user-1", 10, &now)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, "payments", list[0].TagValue)

		actions, err := New(db).GetTagPolicyActions(context.Background(), "project-1", "job", "user-1", map[string]string{"team": "payments"})
		require.NoError(t, err)
		require.Equal(t, []string{"read", "write"}, actions)

		deletedProject, userID, err := New(db).DeleteTagPolicy(context.Background(), "project-1", "tag-policy-1")
		require.NoError(t, err)
		require.Equal(t, "project-1", deletedProject)
		require.Equal(t, "user-1", userID)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		_, _, err = New(db).DeleteTagPolicy(context.Background(), "project-1", "missing")
		require.ErrorIs(t, err, ErrTagPolicyNotFound)

		db.queryFn = func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{}, nil
		}
		actions, err = New(db).GetTagPolicyActions(context.Background(), "project-1", "job", "user-1", map[string]string{"team": "other"})
		require.NoError(t, err)
		require.Nil(t, actions)
	})
}

func TestRBACSeedProjectSystemRolesUnit(t *testing.T) {
	t.Parallel()

	var execs int
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			require.Contains(t, sql, "ON CONFLICT")
			require.Equal(t, "project-1", args[1])
			require.True(t, args[5].(bool))
			execs++
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	require.NoError(t, New(db).SeedProjectSystemRoles(context.Background(), "project-1"))
	require.Equal(t, len(domain.SystemRolePermissions), execs)

	seedErr := errors.New("seed failed")
	db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, seedErr
	}
	err := New(db).SeedProjectSystemRoles(context.Background(), "project-1")
	require.ErrorContains(t, err, "seed system role")
	require.ErrorIs(t, err, seedErr)
}
