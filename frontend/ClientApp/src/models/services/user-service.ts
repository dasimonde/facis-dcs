import type { UserRole } from "@/types/user-role"
import type { UserAllRequest, UserRolesByUserIdRequest } from "../requests/user-request"
import type { UserRolesByUserIdResponse } from "../responses/user-response"
import type { UserProfile } from "../user"

export interface UserService {
  getAllUsers: (request?: UserAllRequest) => Promise<UserProfile[]>
  getRolesByUser: (request: UserRolesByUserIdRequest) => Promise<UserRolesByUserIdResponse>
  getAuthorizedUsersWithRoles: (...roles: [...UserRole[], UserRole | undefined]) => Promise<UserProfile[]>
}
