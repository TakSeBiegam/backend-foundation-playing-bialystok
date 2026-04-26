package graph

import (
	"backend/graph/model"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type permissionAction string

const (
	permissionActionRead   permissionAction = "read"
	permissionActionWrite  permissionAction = "write"
	permissionActionDelete permissionAction = "delete"
)

var managedAdminResources = []model.AdminResource{
	model.AdminResourceDashboard,
	model.AdminResourceOfferPage,
	model.AdminResourceAboutUsPage,
	model.AdminResourceEvents,
	model.AdminResourcePartners,
	model.AdminResourceCatalog,
	model.AdminResourceGallery,
	model.AdminResourceMessages,
	model.AdminResourceUsers,
	model.AdminResourceRolePermissions,
	model.AdminResourceAuditLogs,
}

var managedRoles = []model.Role{
	model.RoleOwner,
	model.RoleAdmin,
	model.RoleModerator,
	model.RoleEditor,
}

func currentRoleFromContext(ctx context.Context) (model.Role, error) {
	rawValue, _ := ctx.Value("callerRole").(string)
	value := strings.ToUpper(strings.TrimSpace(rawValue))

	switch value {
	case "OWNER":
		return model.RoleOwner, nil
	case "ADMIN":
		return model.RoleAdmin, nil
	case "MODERATOR":
		return model.RoleModerator, nil
	case "EDITOR":
		return model.RoleEditor, nil
	case "":
		return "", fmt.Errorf("unauthenticated")
	default:
		return "", fmt.Errorf("invalid caller role")
	}
}

func currentUserIDFromContext(ctx context.Context) (string, error) {
	value, _ := ctx.Value("callerId").(string)
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("unauthenticated")
	}

	return trimmed, nil
}

func normalizeOfferBlockPageKey(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "about-us") {
		return "about-us"
	}

	return "offer"
}

func adminResourceForPageKey(value string) model.AdminResource {
	if normalizeOfferBlockPageKey(value) == "about-us" {
		return model.AdminResourceAboutUsPage
	}

	return model.AdminResourceOfferPage
}

func hasCallerRole(ctx context.Context) bool {
	value, _ := ctx.Value("callerRole").(string)
	return strings.TrimSpace(value) != ""
}

func (r *Resolver) allowPublicReadPermission(ctx context.Context, resource model.AdminResource) error {
	if !hasCallerRole(ctx) {
		return nil
	}

	return r.requirePermission(ctx, resource, permissionActionRead)
}

func (r *Resolver) requirePermission(
	ctx context.Context,
	resource model.AdminResource,
	action permissionAction,
) error {
	role, err := currentRoleFromContext(ctx)
	if err != nil {
		return err
	}

	if role == model.RoleOwner {
		return nil
	}

	permission, err := r.getPermissionForRole(ctx, role, resource)
	if err != nil {
		return err
	}

	allowed := false
	switch action {
	case permissionActionRead:
		allowed = permission.CanRead
	case permissionActionWrite:
		allowed = permission.CanWrite
	case permissionActionDelete:
		allowed = permission.CanDelete
	default:
		return fmt.Errorf("unknown permission action")
	}

	if !allowed {
		return fmt.Errorf("forbidden: insufficient permissions")
	}

	return nil
}

func syntheticPermission(
	role model.Role,
	resource model.AdminResource,
	canRead bool,
	canWrite bool,
	canDelete bool,
) *model.RolePermission {
	return &model.RolePermission{
		Role:      role,
		Resource:  resource,
		CanRead:   canRead,
		CanWrite:  canWrite,
		CanDelete: canDelete,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func (r *Resolver) getPermissionForRole(
	ctx context.Context,
	role model.Role,
	resource model.AdminResource,
) (*model.RolePermission, error) {
	if role == model.RoleOwner {
		return syntheticPermission(role, resource, true, true, true), nil
	}

	row := r.DB.QueryRow(ctx,
		`SELECT can_read, can_write, can_delete, updated_at, updated_by_user_id
		 FROM role_permissions
		 WHERE role = $1 AND resource = $2`,
		roleToDB(role),
		string(resource),
	)

	var updatedAt time.Time
	var updatedByUserID *string
	permission := syntheticPermission(role, resource, false, false, false)

	err := row.Scan(
		&permission.CanRead,
		&permission.CanWrite,
		&permission.CanDelete,
		&updatedAt,
		&updatedByUserID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return permission, nil
		}

		return nil, fmt.Errorf("load role permission: %w", err)
	}

	permission.UpdatedAt = updatedAt.Format(time.RFC3339)
	if updatedByUserID != nil {
		updatedBy, err := r.getUserByID(ctx, *updatedByUserID)
		if err != nil {
			return nil, err
		}
		permission.UpdatedBy = updatedBy
	}

	return permission, nil
}

func (r *Resolver) listPermissionsForRole(
	ctx context.Context,
	role model.Role,
) ([]*model.RolePermission, error) {
	permissionsByResource := make(map[model.AdminResource]*model.RolePermission)

	if role != model.RoleOwner {
		rows, err := r.DB.Query(ctx,
			`SELECT resource, can_read, can_write, can_delete, updated_at, updated_by_user_id
			 FROM role_permissions
			 WHERE role = $1`,
			roleToDB(role),
		)
		if err != nil {
			return nil, fmt.Errorf("list role permissions: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var resourceValue string
			var canRead bool
			var canWrite bool
			var canDelete bool
			var updatedAt time.Time
			var updatedByUserID *string

			if err := rows.Scan(
				&resourceValue,
				&canRead,
				&canWrite,
				&canDelete,
				&updatedAt,
				&updatedByUserID,
			); err != nil {
				return nil, fmt.Errorf("scan role permission: %w", err)
			}

			resource := model.AdminResource(strings.TrimSpace(resourceValue))
			permission := syntheticPermission(role, resource, canRead, canWrite, canDelete)
			permission.UpdatedAt = updatedAt.Format(time.RFC3339)

			if updatedByUserID != nil {
				updatedBy, err := r.getUserByID(ctx, *updatedByUserID)
				if err != nil {
					return nil, err
				}
				permission.UpdatedBy = updatedBy
			}

			permissionsByResource[resource] = permission
		}
	}

	permissions := make([]*model.RolePermission, 0, len(managedAdminResources))
	for _, resource := range managedAdminResources {
		permission, exists := permissionsByResource[resource]
		if !exists {
			allowAll := role == model.RoleOwner
			permission = syntheticPermission(role, resource, allowAll, allowAll, allowAll)
		}

		permissions = append(permissions, permission)
	}

	return permissions, nil
}

func (r *Resolver) listAllRolePermissions(ctx context.Context) ([]*model.RolePermission, error) {
	permissions := make([]*model.RolePermission, 0, len(managedRoles)*len(managedAdminResources))

	for _, role := range managedRoles {
		rolePermissions, err := r.listPermissionsForRole(ctx, role)
		if err != nil {
			return nil, err
		}

		permissions = append(permissions, rolePermissions...)
	}

	return permissions, nil
}
