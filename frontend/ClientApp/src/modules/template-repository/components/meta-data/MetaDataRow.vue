<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    initialName: string
    initialValue: string
    allNames: string[]
    index?: number
    isNew?: boolean
    isActive?: boolean
    isEditable?: boolean
  }>(),
  { isEditable: true },
)

const emit = defineEmits<{
  confirm: [{ name: string; value: string }]
  cancel: []
  delete: []
  rowFocus: []
}>()

const isNew = computed(() => props.isNew === true)
const isEditing = ref(isNew.value)
const localName = ref<string>(props.initialName ?? '')
const localValue = ref<string>(props.initialValue ?? '')

const localNameId = useId()
const localValueId = useId()

watch(
  () => [props.initialName, props.initialValue],
  ([name, value]) => {
    if (!isEditing.value) {
      localName.value = name ?? ''
      localValue.value = value ?? ''
    }
  },
)

const isDuplicate = computed(() => {
  const name = localName.value.trim().toLowerCase()
  if (!name) return false
  return props.allNames.some((n, idx) => {
    if (props.index !== undefined && idx === props.index) return false
    return n.trim().toLowerCase() === name
  })
})

const canConfirm = computed(() => !!localName.value.trim() && !isDuplicate.value)

const isActive = computed(() => props.isActive === true)

function startEdit() {
  isEditing.value = true
  localName.value = props.initialName ?? ''
  localValue.value = props.initialValue ?? ''
}

function onConfirm() {
  if (!canConfirm.value) return
  emit('confirm', {
    name: localName.value.trim(),
    value: localValue.value,
  })
  if (!isNew.value) {
    isEditing.value = false
  }
}

function onCancel() {
  if (isNew.value) {
    emit('cancel')
    return
  }
  isEditing.value = false
  localName.value = props.initialName ?? ''
  localValue.value = props.initialValue ?? ''
  emit('cancel')
}

function onDelete() {
  emit('delete')
}

function onRowClick() {
  emit('rowFocus')
}
</script>

<template>
  <tr
    class="group cursor-pointer border border-transparent hover:bg-base-200/70"
    :class="{ 'border-primary bg-primary/5': isActive }"
    @click="onRowClick"
  >
    <!-- Name -->
    <td class="align-top">
      <div v-if="isEditing" class="flex flex-col gap-1">
        <label :for="localNameId" class="sr-only">Name</label>
        <input
          :id="localNameId"
          v-model="localName"
          type="text"
          class="input-bordered input input-xs w-full"
          placeholder="Name"
          :disabled="!isEditable"
        />
        <p v-if="isDuplicate" class="text-[11px] text-error">Name already exists.</p>
      </div>
      <div v-else class="truncate font-mono text-xs">{{ initialName }}</div>
    </td>

    <!-- Value -->
    <td class="align-top">
      <div v-if="isEditing">
        <label :for="localValueId" class="sr-only">Value</label>
        <input
          :id="localValueId"
          v-model="localValue"
          type="text"
          class="input-bordered input input-xs w-full"
          placeholder="Value"
          :disabled="!isEditable"
        />
      </div>
      <div v-else class="truncate text-xs">{{ initialValue }}</div>
    </td>

    <!-- Actions -->
    <td v-if="isEditable" class="w-32 text-right align-top">
      <!-- Add row: actions always visible -->
      <div v-if="isNew" class="flex justify-end gap-1">
        <button
          v-if="isEditing"
          type="button"
          class="btn btn-xs btn-primary"
          :disabled="!canConfirm"
          @click="onConfirm"
        >
          Add
        </button>
        <button v-if="isEditing" type="button" class="btn btn-outline btn-xs" @click="onCancel">Cancel</button>
      </div>

      <!-- Existing rows: actions hidden until hover or focus within the row -->
      <div
        v-else
        class="flex justify-end gap-1 transition-opacity"
        :class="{
          'opacity-100': isActive || isEditing,
          'opacity-0 group-focus-within:opacity-100 group-hover:opacity-100': !isActive && !isEditing,
        }"
      >
        <template v-if="isEditing">
          <button type="button" class="btn btn-xs btn-primary" :disabled="!canConfirm" @click="onConfirm">
            Confirm
          </button>
          <button type="button" class="btn btn-outline btn-xs" @click="onCancel">Cancel</button>
          <button type="button" class="btn text-error btn-outline btn-xs" @click="onDelete">Delete</button>
        </template>
        <template v-else>
          <button type="button" class="btn btn-outline btn-xs" @click="startEdit">Edit</button>
          <button type="button" class="btn text-error btn-outline btn-xs" @click="onDelete">Delete</button>
        </template>
      </div>
    </td>
  </tr>
</template>
