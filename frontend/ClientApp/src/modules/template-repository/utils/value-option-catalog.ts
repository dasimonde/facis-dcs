import {
  resolveAllowedValues,
  resolveValueConstraintOptions,
} from '@template-repository/utils/value-constraint-catalog'
import type { SemanticValueConstraint, SemanticValueOption } from '@template-repository/models/contract-template'

export type ValueOption = Required<Pick<SemanticValueOption, 'value' | 'label'>> &
  Pick<SemanticValueOption, 'symbol' | 'iri' | 'catalog'>

export interface ValueOptionGroup {
  iri: string
  label: string
  options: readonly ValueOption[]
}

export function resolveValueOptions(constraint?: SemanticValueConstraint): readonly ValueOption[] {
  if (!constraint) return []
  const annotatedOptions = resolveValueConstraintOptions(constraint) ?? []
  if (annotatedOptions.length) {
    return annotatedOptions.map((option) => ({
      ...option,
      label: option.label ?? option.value,
    }))
  }
  return resolveAllowedValues(constraint).map((value) => {
    return {
      value,
      label: value,
    }
  })
}

export function groupValueOptions(options: readonly ValueOption[]): readonly ValueOptionGroup[] {
  const groups = new Map<string, { iri: string; label: string; options: ValueOption[] }>()
  for (const option of options) {
    const iri = option.catalog?.iri ?? ''
    const group = groups.get(iri)
    if (group) group.options.push(option)
    else groups.set(iri, { iri, label: option.catalog?.label ?? '', options: [option] })
  }
  return [...groups.values()]
}

export function formatValueOption(value: unknown, options: readonly ValueOption[]): string {
  const raw = String(value)
  const option = options.find((item) => item.iri === raw || item.value === raw)
  if (!option) return raw
  if (option.symbol) return `${option.symbol} ${option.value}`
  return option.label === option.value ? option.value : `${option.label} (${option.value})`
}
