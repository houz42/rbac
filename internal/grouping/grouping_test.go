package grouping

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"testing"

	"github.com/go-logr/stdr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	. "github.com/supremind/rbac/internal/testdata"
	"github.com/supremind/rbac/persist/fake"
	. "github.com/supremind/rbac/types"
)

func TestGrouping(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "grouping test suit")
}

var _ = Describe("grouping implementation", func() {
	Specify("init grouping polices are created", func() {
		Expect(UserRoles).NotTo(BeEmpty())
		Expect(RoleUsers).NotTo(BeEmpty())
	})

	var groupers = []struct {
		name string
		g    func() grouping
	}{
		{
			name: "synced fat",
			g:    func() grouping { return newSyncedGrouping(newFatGrouping()) },
		},
		{
			name: "synced slim",
			g:    func() grouping { return newSyncedGrouping(newSlimGrouping()) },
		},
		{
			name: "fake persisted",
			g: func() grouping {
				logger := stdr.New(log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile))
				stdr.SetVerbosity(4)

				g, e := newPersistedGrouping(context.Background(), fake.NewGroupingPersister(), logger)
				Expect(e).To(Succeed())
				return g
			},
		},
	}

	for _, tg := range groupers {
		ctor := tg.g

		Describe(tg.name, func() {
			var g grouping

			BeforeEach(func() {
				g = ctor()
				for user, roles := range UserRoles {
					for _, role := range roles {
						Expect(g.Join(user, role)).To(Succeed())
					}
				}
			})

			It("should contain initial users", func() {
				Expect(g.AllMembers()).To(haveExactKeys(
					User("0"), User("1"), User("2"), User("3"), User("4"),
					User("5"), User("6"), User("7"), User("8"), User("9"),
				))
			})

			It("should contain initial roles", func() {
				Expect(g.AllGroups()).To(haveExactKeys(
					Role("2_0"), Role("2_1"),
					Role("3_0"), Role("3_1"), Role("3_2"),
					Role("5_0"), Role("5_1"), Role("5_2"), Role("5_3"), Role("5_4"),
				))
			})

			It("should know roles of user", func() {
				for user, roles := range UserRoles {
					user, roles := user, roles
					Expect(g.GroupsOf(user)).To(haveExactKeys(func() []interface{} {
						is := make([]interface{}, 0, len(roles))
						for _, role := range roles {
							is = append(is, role)
						}
						return is
					}()...))
				}
			})

			It("should know users of roles", func() {
				for role, users := range RoleUsers {
					role, users := role, users
					Expect(g.MembersIn(role)).To(haveExactKeys(func() []interface{} {
						is := make([]interface{}, 0, len(users))
						for _, user := range users {
							is = append(is, user)
						}
						return is
					}()...))
				}
			})

			It("should know user is in role", func() {
				for user, roles := range UserRoles {
					for _, role := range roles {
						user, role := user, role
						Expect(g.IsIn(user, role)).To(BeTrue())
					}
				}
			})

			It("should know user is not in role", func() {
				for _, tc := range []struct {
					user User
					role Role
				}{
					{user: User("1"), role: Role("2_0")},
					{user: User("4"), role: Role("3_0")},
					{user: User("4"), role: Role("3_2")},
					{user: User("6"), role: Role("2_1")},
					{user: User("6"), role: Role("3_1")},
					{user: User("6"), role: Role("3_2")},
				} {
					user, role := tc.user, tc.role
					Expect(g.IsIn(user, role)).To(BeFalse())
				}
			})

			DescribeTable("user leaves role",
				func(user User, role Role) {
					Expect(g.Leave(user, role)).To(Succeed())
					Expect(g.GroupsOf(user)).NotTo(HaveKey(role), fmt.Sprintf("%s should not be in roles of %s", role, user))
					Expect(g.MembersIn(role)).NotTo(HaveKey(user), fmt.Sprintf("%s should not be in users of %s", user, role))
					Expect(g.IsIn(user, role)).NotTo(BeTrue(), fmt.Sprintf("%s should not be in %s", user, role))
				},
				Entry("user 1 leaves role 3_1", User("1"), Role("3_1")),
				Entry("user 7 leaves role 5_2", User("7"), Role("5_2")),
				Entry("user 6 leaves role 3_0", User("6"), Role("3_0")),
			)

			Describe("removing role", func() {
				JustBeforeEach(func() {
					Expect(g.RemoveGroup(Role("3_2"))).To(Succeed())
				})

				It("should remove it from all roles", func() {
					Expect(g.AllGroups()).NotTo(HaveKey(Role("3_2")))
				})

				DescribeTable("should remove it from roles of its users",
					func(user User) {
						Expect(g.GroupsOf(user)).NotTo(HaveKey(Role("3_2")))
					},
					Entry("user 2", User("2")),
					Entry("user 5", User("5")),
					Entry("user 8", User("8")),
				)

				DescribeTable("users should not be in it anymore",
					func(user User) {
						Expect(g.IsIn(user, Role("3_2"))).NotTo(BeTrue())
					},
					Entry("user 2", User("2")),
					Entry("user 5", User("5")),
					Entry("user 8", User("8")),
				)
			})

			Describe("removing user", func() {
				JustBeforeEach(func() {
					Expect(g.RemoveMember(User("2"))).To(Succeed())
				})

				It("should remove it from all users", func() {
					Expect(g.AllMembers()).NotTo(HaveKey(User("2")))
				})

				DescribeTable("should remove it from users of its roles",
					func(role Role) {
						Expect(g.MembersIn(role)).NotTo(HaveKey(User("2")))
					},
					Entry("role 2_0", Role("2_0")),
					Entry("role 3_2", Role("3_2")),
					Entry("role 5_2", Role("5_2")),
				)

				DescribeTable("should remove relationships about it",
					func(role Role) {
						Expect(g.IsIn(User("2"), role)).To(BeFalse())
					},
					Entry("role 2_0", Role("2_0")),
					Entry("role 3_2", Role("3_2")),
					Entry("role 5_2", Role("5_2")),
				)
			})

			Describe("with role-to-role groupings", func() {
				JustBeforeEach(func() {
					Expect(g.Join(Role("2_0"), Role("even"))).To(Succeed())
					Expect(g.Join(Role("2_0"), Role("divisible"))).To(Succeed())
					Expect(g.Join(Role("3_0"), Role("divisible"))).To(Succeed())
					Expect(g.Join(Role("5_0"), Role("divisible"))).To(Succeed())
				})

				DescribeTable("querying direct subjects of role",
					func(role Role, subjects []interface{}) {
						Expect(g.immediateEntitiesIn(role)).To(haveExactKeys(subjects...))
					},
					Entry("users of role 3_0", Role("3_0"), []interface{}{User("0"), User("3"), User("6"), User("9")}),
					Entry("sub roles of divisible", Role("divisible"), []interface{}{Role("2_0"), Role("3_0"), Role("5_0")}),
				)

				DescribeTable("querying direct roles of subject",
					func(ent Entity, roles []interface{}) {
						Expect(g.immediateGroupsOf(ent)).To(haveExactKeys(roles...))
					},
					Entry("roles of user 9", User("9"), []interface{}{Role("2_1"), Role("3_0"), Role("5_4")}),
				)

				DescribeTable("querying users of super role",
					func(role Role, users []interface{}) {
						Expect(g.MembersIn(role)).To(haveExactKeys(users...))
					},
					Entry("even numbers", Role("even"), []interface{}{User("0"), User("2"), User("4"), User("6"), User("8")}),
					Entry("divisible numbers", Role("divisible"),
						[]interface{}{User("0"), User("2"), User("3"), User("4"), User("5"), User("6"), User("8"), User("9")},
					),
				)

				DescribeTable("querying roles of user",
					func(user User, roles []interface{}) {
						Expect(g.GroupsOf(user)).To(haveExactKeys(roles...))
					},
					Entry("roles of user 1", User("1"), []interface{}{Role("2_1"), Role("3_1"), Role("5_1")}),
					Entry("roles of user 4", User("4"), []interface{}{Role("2_0"), Role("3_1"), Role("5_4"), Role("even"), Role("divisible")}),
					Entry("roles of user 9", User("9"), []interface{}{Role("2_1"), Role("3_0"), Role("5_4"), Role("divisible")}),
				)

				Specify("even numbers", func() {
					for _, u := range []int{0, 2, 4, 6, 8} {
						user := User(strconv.Itoa(u))
						Expect(g.IsIn(user, Role("even"))).To(BeTrue())
					}
				})

				Specify("divisible numbers", func() {
					for _, u := range []int{0, 2, 3, 4, 5, 6, 8, 9} {
						user := User(strconv.Itoa(u))
						Expect(g.IsIn(user, Role("divisible"))).To(BeTrue())
					}
				})

				Specify("indivisible numbers", func() {
					for _, u := range []int{1, 7} {
						user := User(strconv.Itoa(u))
						Expect(g.IsIn(user, Role("divisible"))).To(BeFalse())
					}
				})
			})
		})
	}
})
