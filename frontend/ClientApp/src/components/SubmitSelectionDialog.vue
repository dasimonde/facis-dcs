<script setup lang="ts">
import type { SelectedUserRole, UserProfile } from '@/models/user'
import { userService } from '@/services/user-service'
import type { UserRole } from '@/types/user-role'
import { toProperCase } from '@/utils/string'
import { computed, ref, useTemplateRef, type Ref } from 'vue'

const props = defineProps<{
  dialogType: 'template' | 'contract'
}>()

const emit = defineEmits<{
  submit: [value: SelectedUserRole[]]
}>()

const userSelectionModal = useTemplateRef('user-selection-modal')

const users: Ref<UserProfile[]> = ref([])
const isLoading = ref(true)

const selectedUsers = ref<Record<string, boolean>>({})
const selectedRole = ref<Record<string, UserRole>>({})

const roles = computed(() => {
  const roleMap: Record<typeof props.dialogType, { review: UserRole; approve: UserRole; negotiate?: UserRole }> = {
    template: { review: 'TEMPLATE_REVIEWER', approve: 'TEMPLATE_APPROVER' },
    contract: { review: 'CONTRACT_REVIEWER', approve: 'CONTRACT_APPROVER', negotiate: 'CONTRACT_NEGOTIATOR' },
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
    users.value.filter((user) => selectedRole.value[user.id] === approveRole.value).length === 1 &&
    users.value.some((user) => selectedRole.value[user.id] === reviewRole.value) &&
    ((props.dialogType === 'template' && !negotiateRole.value) ||
      (props.dialogType === 'contract' &&
        users.value.some((user) => selectedRole.value[user.id] === negotiateRole.value)))
  )
})
const isSubmitDisabled = computed(() => !hasValidSelection.value || !allSelectedUsersHaveRoles.value)

async function openModal() {
  userSelectionModal.value?.showModal()
  users.value = await userService.getAuthorizedUsersWithRoles(approveRole.value, reviewRole.value, negotiateRole.value)
  isLoading.value = false
}

function onModalSubmit() {
  if (isSubmitDisabled.value) return
  const result = users.value
    .filter((user) => selectedUsers.value[user.id] && selectedRole.value[user.id])
    .map((user) => ({ user, role: selectedRole.value[user.id]! }))
  emit('submit', result)
  onModalClose()
}

function onModalClose() {
  userSelectionModal.value?.close()
  selectedUsers.value = {}
  selectedRole.value = {}
  isLoading.value = true
}

function isRoleDisabled(role: UserRole, userId: string) {
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
    : 'Select one Approver, at least one Reviewer and at least one Negotiator'
})
</script>

<template>
  <button :="$attrs" @click="openModal">Submit</button>
  <dialog ref="user-selection-modal" class="modal modal-bottom sm:modal-middle transition-none" @close="onModalClose">
    <div class="modal-box flex flex-col max-h-2/3">
      <h3 class="text-lg font-bold">
        User Selection for {{ dialogType === 'template' ? 'Template' : 'Contract' }} Submission
      </h3>
      <p class="text-sm py-4">
        {{ roleInfoText }}
      </p>
      <div class="overflow-y-auto grow">
        <div v-if="isLoading">Loading...</div>
        <ul v-else class="list">
          <li v-for="user in users" :key="user.id" class="list-row border border-secondary mb-1 py-2">
            <label class="label list-col-grow">
              <input
                v-model="selectedUsers[user.id]"
                @change="onCheckboxChange($event, user.id)"
                type="checkbox"
                class="checkbox checkbox-primary mr-4"
              />
              {{ `${user.firstName} ${user.lastName}` }}
            </label>
            <select
              v-model="selectedRole[user.id]"
              class="select select-sm sm:select-md select-primary"
              :disabled="!selectedUsers[user.id]"
            >
              <option selected :value="selectedRole['']">No role</option>
              <option v-for="role in user.roleIds" :key="role" :value="role" :disabled="isRoleDisabled(role, user.id)">
                {{ toProperCase(role) }}
              </option>
            </select>
          </li>
        </ul>
      </div>
      <div class="modal-action">
        <div v-if="isSubmitDisabled" class="text-sm text-error flex items-center">
          <span v-if="!hasSelectedUsers">Select at least one user</span>
          <span v-else-if="!allSelectedUsersHaveRoles">Assign a role to all selected users</span>
          <span v-else>{{ roleInfoText }}</span>
        </div>
        <button @click="onModalSubmit" :disabled="isSubmitDisabled" class="btn btn-primary">Apply</button>
        <button @click="onModalClose" class="btn btn-secondary">Cancel</button>
      </div>
    </div>
  </dialog>
</template>
