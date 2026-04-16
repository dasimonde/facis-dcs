import type { UserRole } from "@/types/user-role"

export interface UserProfile {
  participantId: string
  firstName: string
  lastName: string
  email: string
  roleIds?: UserRole[]
  id: string
  username: string
}

export interface SelectedUserRole {
  user: UserProfile
  role: UserRole
}
