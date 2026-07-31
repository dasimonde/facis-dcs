export type ParameterType = 'string' | 'decimal' | 'integer' | 'boolean' | 'date' | 'enum'

export type DcsOperator =
  | 'odrl:eq'
  | 'odrl:neq'
  | 'odrl:gt'
  | 'odrl:gteq'
  | 'odrl:lt'
  | 'odrl:lteq'
  | 'odrl:isAnyOf'
  | 'odrl:isNoneOf'
  | 'odrl:hasPart'
  | 'dcs:between'
  | 'dcs:matchesRegex'

export interface UiMetadata {
  label?: string
  description?: string
  input?: 'text' | 'textarea' | 'number' | 'date' | 'checkbox' | 'select' | 'country' | 'url'
  placeholder?: string
  suffix?: string
  order?: number
  group?: string
}
