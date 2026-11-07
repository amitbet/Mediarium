import { baseName } from './replaceFile.ts'
import { t } from './i18n.ts'

// A download that finished but was not imported because the file the title
// already had was kept (see the queue row). These say which file that is and
// what became of the downloaded files, which the server records as
// "deleted", "seeding" or "client".

export function keptDataText(data: string): string {
  switch (data) {
    case 'seeding':
      return t('It is still seeding, so its downloaded files stay until seeding is done.')
    case 'client':
      return t('It stays in your download client, which keeps its files.')
    default:
      return t('Its downloaded files were deleted.')
  }
}

// The kept file by name; the full path is in the row's details.
export function keptFileLine(path: string): string {
  return t('Kept: {file}', { file: baseName(path) })
}
