export const parseClassList = (value: string | null): string[] => {
  if (!value) {
    return []
  }

  return value
    .split(' ')
    .map((entry: string): string => entry.trim())
    .filter((entry: string): boolean => entry.length > 0)
}
