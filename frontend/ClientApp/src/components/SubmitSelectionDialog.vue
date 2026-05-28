<script setup lang="ts">
import type { SelectedUserRole, UserProfile } from '@/models/user'
import { userService } from '@/services/user-service'
import type { UserRole } from '@/types/user-role'
import { toProperCase } from '@/utils/string'
import { computed, ref, useTemplateRef, type Ref } from 'vue'

interface User extends UserProfile {
  availableRoles: (UserRole | 'CONTRACT_NEGOTIATOR')[]
}

const props = defineProps<{
  dialogType: 'template' | 'contract'
}>()

const emit = defineEmits<{
  submit: [value: SelectedUserRole[]]
}>()

const userSelectionModal = useTemplateRef('user-selection-modal')

const users: Ref<User[]> = ref([])
const isLoading = ref(true)

const selectedUsers = ref<Record<string, boolean>>({})
const selectedRole = ref<Record<string, UserRole | 'CONTRACT_NEGOTIATOR'>>({})

const roles = computed(() => {
  const roleMap: Record<typeof props.dialogType, { review: UserRole; approve: UserRole; negotiate?: UserRole }> = {
    template: { review: 'TEMPLATE_REVIEWER', approve: 'TEMPLATE_APPROVER' },
    contract: { review: 'CONTRACT_REVIEWER', approve: 'CONTRACT_APPROVER', negotiate: 'CONTRACT_CREATOR' },
  }
  return roleMap[props.dialogType]
})
const reviewRole = computed(() => roles.value.review)
const approveRole = computed(() => roles.value.approve)
const negotiateRole = computed(() => roles.value.negotiate)

const hasSelectedUsers = computed(() => {
  return users.value.some((user) => selectedUsers.value[user.id])
})
const allSelectedUsersHaveRoles = computed(() => {
  return users.value.every((user) => !selectedUsers.value[user.id] || selectedRole.value[user.id])
})
const hasValidSelection = computed(() => {
  return (
    users.value.some((user) => selectedRole.value[user.id] === approveRole.value) &&
    users.value.some((user) => selectedRole.value[user.id] === reviewRole.value) &&
    ((props.dialogType === 'template' && !negotiateRole.value) ||
      (props.dialogType === 'contract' &&
        users.value.some((user) => selectedRole.value[user.id] === 'CONTRACT_NEGOTIATOR')))
  )
})
const isSubmitDisabled = computed(() => !hasValidSelection.value || !allSelectedUsersHaveRoles.value)

async function openModal() {
  userSelectionModal.value?.showModal()
  const receivedUsers = await userService.getAuthorizedUsersWithRoles(
    approveRole.value,
    reviewRole.value,
    negotiateRole.value,
  )
  users.value = receivedUsers.map((user) => ({
    ...user,
    availableRoles:
      user.roleIds?.reduce<(UserRole | 'CONTRACT_NEGOTIATOR')[]>((acc, role) => {
        const newRole = 'CONTRACT_NEGOTIATOR'
        if (role === 'CONTRACT_CREATOR') {
          acc.push(newRole)
        } else if (role === 'CONTRACT_REVIEWER') {
          acc.push(role, newRole)
        } else {
          acc.push(role)
        }
        return [...new Set(acc)]
      }, []) ?? [],
  }))

  isLoading.value = false
}

function onModalSubmit() {
  if (isSubmitDisabled.value) return
  const result = users.value
    .filter((user) => selectedUsers.value[user.id] && selectedRole.value[user.id])
    .map(({ availableRoles: _, ...user }): SelectedUserRole => ({ user: user, role: selectedRole.value[user.id]! }))
  emit('submit', result)
  onModalClose()
}

function onModalClose() {
  userSelectionModal.value?.close()
  selectedUsers.value = {}
  selectedRole.value = {}
  isLoading.value = true
}

function isRoleDisabled(role: UserRole | 'CONTRACT_NEGOTIATOR', userId: string) {
  if (props.dialogType === 'contract') return false
  const roles = Object.values(selectedRole.value)
  return role === approveRole.value && selectedRole.value[userId] !== role && roles.includes(role)
}

function onCheckboxChange(event: Event, userId: string) {
  const checked = (event.target as HTMLInputElement).checked
  if (!checked) {
    delete selectedRole.value[userId]
  }
}

const roleInfoText = computed(() => {
  return props.dialogType === 'template'
    ? 'Select one Approver and at least one Reviewer'
    : 'Select at least one Approver, at least one Reviewer and at least one Negotiator'
})
</script>

<template>
  <button :="$attrs" @click="openModal">Submit</button>
  <dialog ref="user-selection-modal" class="modal modal-bottom transition-none sm:modal-middle" @close="onModalClose">
    <div class="modal-box flex max-h-2/3 flex-col">
      <h3 class="text-lg font-bold">
        User Selection for {{ dialogType === 'template' ? 'Template' : 'Contract' }} Submission
      </h3>
      <p class="py-4 text-sm">
        {{ roleInfoText }}
      </p>
      <div class="grow overflow-y-auto">
        <div v-if="isLoading">Loading...</div>
        <ul v-else class="list">
          <li v-for="user in users" :key="user.id" class="list-row mb-1 border border-secondary py-2">
            <label class="list-col-grow label">
              <input
                v-model="selectedUsers[user.id]"
                type="checkbox"
                class="checkbox mr-4 checkbox-primary"
                @change="onCheckboxChange($event, user.id)"
              />
              {{ `${user.firstName} ${user.lastName}` }}
            </label>
            <select
              v-model="selectedRole[user.id]"
              class="select select-sm select-primary sm:select-md"
              :disabled="!selectedUsers[user.id]"
            >
              <option selected :value="selectedRole['']">No role</option>
              <option
                v-for="role in user.availableRoles"
                :key="role"
                :value="role"
                :disabled="isRoleDisabled(role, user.id)"
              >
                {{ toProperCase(role) }}
              </option>
            </select>
          </li>
        </ul>
      </div>
      <div class="modal-action">
        <div v-if="isSubmitDisabled" class="flex items-center text-sm text-error">
          <span v-if="!hasSelectedUsers">Select at least one user</span>
          <span v-else-if="!allSelectedUsersHaveRoles">Assign a role to all selected users</span>
          <span v-else>{{ roleInfoText }}</span>
        </div>
        <button class="btn btn-outline" @click="onModalClose">Cancel</button>
        <button :disabled="isSubmitDisabled" class="btn btn-primary" @click="onModalSubmit">Apply</button>
      </div>
    </div>
  </dialog>
</template>
