export function toProperCase(text: string) {
  return text
    .split('_')
    .map((word) => word.charAt(0).toLocaleUpperCase() + word.slice(1).toLocaleLowerCase())
    .join(' ')
}
