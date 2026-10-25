import { EntityActionsMenu, type EntityAction } from '@/components/ui/entity-actions-menu'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { useI18n } from '@/providers/I18nProvider'

type RoleRow = { id: number; name: string; description?: string; usersCount: number }

export function RolesTable<R extends RoleRow>({ roles, actions, usersLabel }: { roles: R[]; actions: (r: R) => EntityAction[]; usersLabel: (count: number) => string }) {
  const { t } = useI18n()
  return (
    <>
      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              <Th>{t('roles.name')}</Th>
              <Th>{t('roles.description')}</Th>
              <Th>{t('settings.users')}</Th>
              <Th className="w-16">
                <span className="sr-only">{t('cm.actions')}</span>
              </Th>
            </tr>
          </thead>
          <tbody>
            {roles.map((r) => (
              <Tr key={r.id}>
                <Td className="font-medium">{r.name}</Td>
                <Td className="text-muted">{r.description}</Td>
                <Td>{r.usersCount}</Td>
                <Td className="text-right">
                  <EntityActionsMenu actions={actions(r)} label={t('cm.actions')} />
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </div>
      <ul className="flex flex-col gap-3 md:hidden">
        {roles.map((r) => (
          <li key={r.id} className="flex items-start justify-between gap-3 rounded-lg border border-border bg-surface p-4 shadow-sm">
            <div className="min-w-0">
              <p className="font-medium">{r.name}</p>
              <p className="text-sm text-muted">{r.description}</p>
              <p className="mt-1 text-xs text-muted">{usersLabel(r.usersCount)}</p>
            </div>
            <EntityActionsMenu actions={actions(r)} label={t('cm.actions')} />
          </li>
        ))}
      </ul>
    </>
  )
}
