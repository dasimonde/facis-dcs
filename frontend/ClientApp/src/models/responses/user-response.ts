import type { UserRole } from '@/types/user-role'
import type { UserProfile } from '../user'

export interface UserAllResponse {
  totalCount: number
  items: UserProfile[]
}

export type UserRolesByUserIdResponse = UserRole[]
