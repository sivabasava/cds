package group

import (
	"fmt"
	"strings"

	"github.com/go-gorp/gorp"

	"github.com/ovh/cds/sdk"
)

// DeleteGroupAndDependencies deletes group and all subsequent group_project, pipeline_project
func DeleteGroupAndDependencies(db gorp.SqlExecutor, group *sdk.Group) error {
	if err := DeleteGroupUserByGroup(db, group); err != nil {
		return sdk.WrapError(err, "deleteGroupAndDependencies: Cannot delete group user %s", group.Name)
	}

	if err := deleteGroupProjectByGroup(db, group); err != nil {
		return sdk.WrapError(err, "deleteGroupAndDependencies: Cannot delete group project %s", group.Name)
	}

	if err := deleteGroup(db, group); err != nil {
		return sdk.WrapError(err, "deleteGroupAndDependencies: Cannot delete group %s", group.Name)
	}

	// TODO EVENT Send event for all dependencies

	return nil
}

// CheckUserInGroup verivies that user is in given group
func CheckUserInGroup(db gorp.SqlExecutor, groupID, userID int64) (bool, error) {
	query := `SELECT COUNT(user_id) FROM group_user WHERE group_id = $1 AND user_id = $2`

	var nb int64
	err := db.QueryRow(query, groupID, userID).Scan(&nb)
	if err != nil {
		return false, err
	}

	if nb == 1 {
		return true, nil
	}

	return false, nil
}

// DeleteUserFromGroup remove user from group
func DeleteUserFromGroup(db gorp.SqlExecutor, groupID, userID int64) error {
	// Check if there are admin left
	var isAdm bool
	err := db.QueryRow(`SELECT group_admin FROM "group_user" WHERE group_id = $1 AND user_id = $2`, groupID, userID).Scan(&isAdm)
	if err != nil {
		return err
	}

	if isAdm {
		var nbAdm int
		err = db.QueryRow(`SELECT COUNT(id) FROM "group_user" WHERE group_id = $1 AND group_admin = true`, groupID).Scan(&nbAdm)
		if err != nil {
			return err
		}

		if nbAdm <= 1 {
			return sdk.ErrNotEnoughAdmin
		}
	}

	query := `DELETE FROM group_user WHERE group_id=$1 AND user_id=$2`
	_, err = db.Exec(query, groupID, userID)
	return err
}

// InsertUserInGroup insert user in group
func InsertUserInGroup(db gorp.SqlExecutor, groupID, userID int64, admin bool) error {
	query := `INSERT INTO group_user (group_id,user_id,group_admin) VALUES($1,$2,$3)`
	_, err := db.Exec(query, groupID, userID, admin)
	return err
}

// CheckUserInDefaultGroup insert user in default group
func CheckUserInDefaultGroup(db gorp.SqlExecutor, userID int64) error {
	if DefaultGroup != nil && DefaultGroup.ID != 0 {
		inGroup, err := CheckUserInGroup(db, DefaultGroup.ID, userID)
		if err != nil {
			return err
		}
		if !inGroup {
			return InsertUserInGroup(db, DefaultGroup.ID, userID, false)
		}
	}
	return nil
}

// DeleteGroupUserByGroup Delete all user from a group
func DeleteGroupUserByGroup(db gorp.SqlExecutor, group *sdk.Group) error {
	query := `DELETE FROM group_user WHERE group_id=$1`
	_, err := db.Exec(query, group.ID)
	return err
}

// UpdateGroup updates group informations in database
func UpdateGroup(db gorp.SqlExecutor, g *sdk.Group, oldName string) error {
	rx := sdk.NamePatternRegex
	if !rx.MatchString(g.Name) {
		return sdk.NewError(sdk.ErrInvalidName, fmt.Errorf("Invalid group name. It should match %s", sdk.NamePattern))
	}

	query := `UPDATE "group" set name=$1 WHERE name=$2`
	_, err := db.Exec(query, g.Name, oldName)

	if err != nil && strings.Contains(err.Error(), "idx_group_name") {
		return sdk.ErrGroupExists
	}

	return err
}

// Insert given group into database.
func Insert(db gorp.SqlExecutor, g *sdk.Group) error {
	rx := sdk.NamePatternRegex
	if !rx.MatchString(g.Name) {
		return sdk.NewError(sdk.ErrInvalidName, fmt.Errorf("Invalid group name. It should match %s", sdk.NamePattern))
	}

	query := `INSERT INTO "group" (name) VALUES($1) RETURNING id`
	err := db.QueryRow(query, g.Name).Scan(&g.ID)
	return sdk.WithStack(err)
}

// LoadGroupByProject retrieves all groups related to project
func LoadGroupByProject(db gorp.SqlExecutor, project *sdk.Project) error {
	query := `
    SELECT "group".id, "group".name, project_group.role
    FROM "group"
	  JOIN project_group ON project_group.group_id = "group".id
    WHERE project_group.project_id = $1
    ORDER BY "group".name ASC
  `
	rows, err := db.Query(query, project.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var group sdk.Group
		var perm int
		if err := rows.Scan(&group.ID, &group.Name, &perm); err != nil {
			return err
		}
		project.ProjectGroups = append(project.ProjectGroups, sdk.GroupPermission{
			Group:      group,
			Permission: perm,
		})
	}
	return nil
}

func deleteGroup(db gorp.SqlExecutor, g *sdk.Group) error {
	query := `DELETE FROM "group" WHERE id=$1`
	_, err := db.Exec(query, g.ID)
	return err
}

// SetUserGroupAdmin allows a user to perform operations on given group
func SetUserGroupAdmin(db gorp.SqlExecutor, groupID int64, userID int64) error {
	query := `UPDATE "group_user" SET group_admin = true WHERE group_id = $1 AND user_id = $2`

	res, errE := db.Exec(query, groupID, userID)
	if errE != nil {
		return errE
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected != 1 {
		return fmt.Errorf("cannot set user %d group admin of %d", userID, groupID)
	}

	return nil
}

// RemoveUserGroupAdmin remove the privilege to perform operations on given group
func RemoveUserGroupAdmin(db gorp.SqlExecutor, groupID int64, userID int64) error {
	query := `UPDATE "group_user" SET group_admin = false WHERE group_id = $1 AND user_id = $2`
	_, err := db.Exec(query, groupID, userID)
	return err
}
