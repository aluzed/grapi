import { useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, CheckSquare, ChevronRight, File, Folder, FolderInput, FolderPlus, Link2, Pencil, Trash2, Upload, X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { EntityActionsMenu } from '@/components/ui/entity-actions-menu'
import { SelectField, TextField } from '@/components/ui/field'
import { api, ApiError, requestWithMeta, upload } from '@/lib/api'
import { cn } from '@/lib/cn'
import { showError } from '@/lib/form'
import { can } from '@/lib/permissions'
import type { MediaFile, MediaFolder, Pagination } from '@/lib/types'
import { useAuth } from '@/providers/AuthProvider'
import { useConfirm } from '@/providers/ConfirmProvider'
import { useI18n } from '@/providers/I18nProvider'

type Crumb = { id: number | null; name: string }

export const thumbUrl = (f: MediaFile) => f.formats?.thumbnail?.url ?? f.url

export function Thumb({ file, className }: { file: MediaFile; className?: string }) {
  if (file.mime.startsWith('image/')) return <img src={thumbUrl(file)} alt={file.alternativeText ?? file.name} loading="lazy" className={cn('h-full w-full object-cover', className)} />
  return (
    <span className={cn('flex h-full w-full flex-col items-center justify-center gap-1 text-muted', className)}>
      <File className="h-8 w-8" aria-hidden />
      <span className="text-xs uppercase">{file.ext.replace('.', '')}</span>
    </span>
  )
}

function NewFolder({ parent, folder, onClose }: { parent: number | null; folder?: MediaFolder; onClose: () => void }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [name, setName] = useState(folder?.name ?? '')
  const [error, setError] = useState<string>()
  const create = useMutation({
    mutationFn: () => (folder ? api.put(`/upload/folders/${folder.id}`, { name }) : api.post('/upload/folders', { name, parent })),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['media'] })
      onClose()
    },
    onError: (err) => {
      if (err instanceof ApiError) setError(err.fieldErrors()[0]?.message ?? err.message)
      else showError(err)
    },
  })
  return (
    <Dialog
      open
      title={folder ? t('media.renameFolder') : t('media.newFolder')}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={
        <>
          <Button variant="outline" title={t('common.cancel')} onClick={onClose} className="w-full md:w-auto">
            {t('common.cancel')}
          </Button>
          <Button title={folder ? t('common.save') : t('media.create')} icon={<FolderPlus className="h-4 w-4" />} loading={create.isPending} onClick={() => create.mutate()} className="w-full md:w-auto">
            {folder ? t('common.save') : t('media.create')}
          </Button>
        </>
      }
    >
      <TextField label={t('media.folderName')} value={name} error={error} autoFocus onChange={(e) => setName(e.target.value)} />
    </Dialog>
  )
}

function UrlDialog({ folder, onClose }: { folder: number | null; onClose: () => void }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [urls, setUrls] = useState('')
  const [error, setError] = useState<string>()
  const send = useMutation({
    mutationFn: () =>
      api.post<unknown[]>('/upload/actions/upload-from-urls', {
        urls: urls
          .split('\n')
          .map((u) => u.trim())
          .filter(Boolean),
        folder,
      }),
    onSuccess: () => {
      toast.success(t('media.imported'))
      void queryClient.invalidateQueries({ queryKey: ['media'] })
      onClose()
    },
    onError: (e) => {
      if (e instanceof ApiError) setError(e.fieldErrors().map((f) => f.message).join(' ; ') || e.message)
      showError(e)
    },
  })
  return (
    <Dialog
      open
      title={t('media.fromUrl')}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={
        <>
          <Button variant="outline" title={t('common.cancel')} onClick={onClose} className="w-full md:w-auto">
            {t('common.cancel')}
          </Button>
          <Button title={t('media.import')} icon={<Link2 className="h-4 w-4" />} loading={send.isPending} disabled={!urls.trim()} onClick={() => send.mutate()} className="w-full md:w-auto">
            {t('media.import')}
          </Button>
        </>
      }
    >
      <label className="flex flex-col gap-1.5 text-sm">
        <span className="font-medium">{t('media.urls')}</span>
        <textarea
          value={urls}
          rows={4}
          placeholder="https://"
          onChange={(e) => setUrls(e.target.value)}
          className="rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none"
        />
        <span className="text-xs text-muted">{t('media.urlsHelp')}</span>
        {error && <span className="text-xs text-destructive">{error}</span>}
      </label>
    </Dialog>
  )
}

function MoveDialog({ ids, folder, onClose }: { ids: number[]; folder?: MediaFolder; onClose: () => void }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const folders = useQuery({ queryKey: ['media', 'folders', 'all'], queryFn: () => api.get<MediaFolder[]>('/upload/folders?all=true') })
  const [dest, setDest] = useState('root')
  const move = useMutation({
    mutationFn: () =>
      api.post<{ files: number; folders: number }>('/upload/actions/bulk-move', { fileIds: ids, folderIds: folder ? [folder.id] : [], destination: dest === 'root' ? null : Number(dest) }),
    onSuccess: (r) => {
      toast.success(folder ? t('media.folderMoved', { name: folder.name }) : t('media.moved', { count: String(r.files) }))
      void queryClient.invalidateQueries({ queryKey: ['media'] })
      onClose()
    },
    onError: (e) => showError(e),
  })
  return (
    <Dialog
      open
      title={folder ? t('media.moveFolder', { name: folder.name }) : t('media.move')}
      onClose={onClose}
      closeLabel={t('common.cancel')}
      footer={
        <>
          <Button variant="outline" title={t('common.cancel')} onClick={onClose} className="w-full md:w-auto">
            {t('common.cancel')}
          </Button>
          <Button title={t('media.move')} icon={<FolderInput className="h-4 w-4" />} loading={move.isPending} onClick={() => move.mutate()} className="w-full md:w-auto">
            {t('media.move')}
          </Button>
        </>
      }
    >
      <SelectField label={t('media.moveTo')} value={dest} onChange={(e) => setDest(e.target.value)}>
        <option value="root">{t('media.root')}</option>
        {folders.data?.filter((f) => !folder || (f.path !== folder.path && !f.path.startsWith(`${folder.path}/`))).map((f) => (
          <option key={f.id} value={f.id}>
            {'· '.repeat(Math.max(0, f.path.split('/').length - 2))}
            {f.name}
          </option>
        ))}
      </SelectField>
    </Dialog>
  )
}

export type BrowserProps = {
  // onOpen shows a file (page mode); in pick mode a click toggles the selection.
  onOpen?: (f: MediaFile) => void
  pick?: { multiple: boolean; selected: MediaFile[]; onChange: (files: MediaFile[]) => void; allowedTypes?: string[] }
}

export function MediaBrowser({ onOpen, pick }: BrowserProps) {
  const { t } = useI18n()
  const { permissions } = useAuth()
  const queryClient = useQueryClient()
  const [path, setPath] = useState<Crumb[]>([{ id: null, name: t('media.root') }])
  const [q, setQ] = useState('')
  // allowedTypes are plural in Strapi schemas (images, videos, audios, files).
  const initialType = pick?.allowedTypes?.length === 1 ? ({ images: 'image', videos: 'video', audios: 'audio' }[pick.allowedTypes[0]!] ?? '') : ''
  const [type, setType] = useState(initialType)
  const [page, setPage] = useState(1)
  const [creating, setCreating] = useState(false)
  const [renaming, setRenaming] = useState<MediaFolder | null>(null)
  const [dragging, setDragging] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const [chosen, setChosen] = useState<number[]>([])
  const [moving, setMoving] = useState(false)
  const [movingFolder, setMovingFolder] = useState<MediaFolder | null>(null)
  const [importing, setImporting] = useState(false)
  const confirm = useConfirm()
  const canUpdate = can(permissions, 'plugin::upload.assets.update')
  const input = useRef<HTMLInputElement>(null)
  const folder = path[path.length - 1]!.id
  const folderParam = folder === null ? 'root' : String(folder)
  const params = new URLSearchParams({ folder: folderParam, page: String(page), pageSize: '24' })
  if (q) params.set('_q', q)
  if (type) params.set('type', type)
  const files = useQuery({
    queryKey: ['media', 'files', params.toString()],
    queryFn: () => requestWithMeta<MediaFile[], { pagination: Pagination }>(`/upload/files?${params}`),
    placeholderData: (prev) => prev,
  })
  const folders = useQuery({ queryKey: ['media', 'folders', folderParam], queryFn: () => api.get<MediaFolder[]>(`/upload/folders?parent=${folderParam}`), enabled: !q })
  const canUpload = can(permissions, 'plugin::upload.assets.create')

  const send = useMutation({
    mutationFn: (list: File[]) => {
      const form = new FormData()
      for (const f of list) form.append('files', f)
      return upload<MediaFile[]>(`/upload?folder=${folderParam}`, form)
    },
    onSuccess: (res) => {
      toast.success(t('media.uploaded', { count: String(res.length) }))
      void queryClient.invalidateQueries({ queryKey: ['media'] })
    },
    onError: (err) => showError(err, undefined, t('common.error')),
  })
  const onFiles = (list: FileList | null) => {
    if (list && list.length > 0) send.mutate([...list])
  }

  const removeFolder = useMutation({
    mutationFn: (f: MediaFolder) => api.del(`/upload/folders/${f.id}`),
    onSuccess: () => {
      toast.success(t('media.folderDeleted'))
      void queryClient.invalidateQueries({ queryKey: ['media'] })
    },
    onError: (e) => showError(e),
  })
  const bulkDelete = useMutation({
    mutationFn: (ids: number[]) => api.post<{ files: number }>('/upload/actions/bulk-delete', { fileIds: ids }),
    onSuccess: (r) => {
      toast.success(t('media.bulkDeleted', { count: String(r.files) }))
      setChosen([])
      void queryClient.invalidateQueries({ queryKey: ['media'] })
    },
    onError: (e) => showError(e),
  })
  const selectedIds = new Set(pick ? pick.selected.map((f) => f.id) : chosen)
  const click = (f: MediaFile) => {
    if (!pick && selecting) return setChosen((c) => (c.includes(f.id) ? c.filter((x) => x !== f.id) : [...c, f.id]))
    if (!pick) return onOpen?.(f)
    if (selectedIds.has(f.id)) pick.onChange(pick.selected.filter((x) => x.id !== f.id))
    else pick.onChange(pick.multiple ? [...pick.selected, f] : [f])
  }
  const list = files.data?.data ?? []
  const pagination = files.data?.meta.pagination

  return (
    <div
      className={cn('flex flex-col gap-4', dragging && 'rounded-lg outline-2 outline-dashed outline-primary')}
      onDragOver={(e) => {
        if (!canUpload) return
        e.preventDefault()
        setDragging(true)
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault()
        setDragging(false)
        if (canUpload) onFiles(e.dataTransfer.files)
      }}
    >
      <nav aria-label={t('media.folder')} className="flex flex-wrap items-center gap-1 text-sm">
        {path.map((c, i) => (
          <span key={`${c.id}`} className="flex items-center gap-1">
            {i > 0 && <ChevronRight className="h-4 w-4 text-muted" aria-hidden />}
            {i === path.length - 1 ? (
              <span className="font-medium">{c.name}</span>
            ) : (
              <button type="button" title={c.name} onClick={() => setPath(path.slice(0, i + 1))} className="text-primary hover:underline">
                {c.name}
              </button>
            )}
          </span>
        ))}
      </nav>
      <div className="flex flex-col gap-3 xl:flex-row xl:items-end xl:justify-between">
        <div className="flex flex-col gap-3 md:flex-row md:items-end">
          <input
            type="search"
            value={q}
            placeholder={t('media.search')}
            aria-label={t('media.search')}
            onChange={(e) => {
              setQ(e.target.value)
              setPage(1)
            }}
            className="w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none md:w-64"
          />
          <SelectField label={t('media.type')} value={type} onChange={(e) => setType(e.target.value)} className="md:w-40">
            <option value="">{t('media.type.all')}</option>
            {(['image', 'video', 'audio', 'file'] as const).map((k) => (
              <option key={k} value={k}>
                {t(`media.type.${k}`)}
              </option>
            ))}
          </SelectField>
        </div>
        {(canUpload || (!pick && canUpdate)) && (
          <div className="flex flex-col gap-2 md:flex-row md:flex-wrap *:whitespace-nowrap">
            {!pick && canUpdate && (
              <Button
                variant="outline"
                title={selecting ? t('media.cancelSelection') : t('media.selectMode')}
                icon={selecting ? <X className="h-4 w-4" /> : <CheckSquare className="h-4 w-4" />}
                onClick={() => {
                  setSelecting(!selecting)
                  setChosen([])
                }}
                className="w-full md:w-auto"
              >
                {selecting ? t('media.cancelSelection') : t('media.selectMode')}
              </Button>
            )}
            {canUpload && (
            <Button variant="outline" title={t('media.newFolder')} icon={<FolderPlus className="h-4 w-4" />} onClick={() => setCreating(true)} className="w-full md:w-auto">
              {t('media.newFolder')}
            </Button>
            )}
            {canUpload && (
            <Button title={t('media.upload')} icon={<Upload className="h-4 w-4" />} loading={send.isPending} onClick={() => input.current?.click()} className="w-full md:w-auto">
              {send.isPending ? t('media.uploading') : t('media.upload')}
            </Button>
            )}
            {canUpload && (
              <Button variant="outline" title={t('media.fromUrl')} icon={<Link2 className="h-4 w-4" />} onClick={() => setImporting(true)} className="w-full md:w-auto">
                {t('media.fromUrl')}
              </Button>
            )}
            {canUpload && <input ref={input} type="file" multiple hidden onChange={(e) => onFiles(e.target.files)} />}
          </div>
        )}
      </div>
      {canUpload && <p className="hidden text-xs text-muted md:block">{t('media.drop')}</p>}
      {!pick && selecting && chosen.length > 0 && (
        <div className="flex flex-col gap-2 rounded-lg border border-primary/30 bg-primary/5 p-3 md:flex-row md:items-center md:justify-between">
          <span className="text-sm font-medium">{t('media.selected', { count: String(chosen.length) })}</span>
          <div className="flex flex-col gap-2 md:flex-row">
            <Button variant="outline" title={t('media.move')} icon={<FolderInput className="h-4 w-4" />} onClick={() => setMoving(true)} className="w-full md:w-auto">
              {t('media.move')}
            </Button>
            <Button
              variant="destructive-outline"
              title={t('media.delete')}
              icon={<Trash2 className="h-4 w-4" />}
              className="w-full md:w-auto"
              onClick={async () => {
                if (await confirm({ title: t('media.delete'), message: t('media.bulkDeleteConfirm', { count: String(chosen.length) }), confirmLabel: t('media.delete'), destructive: true })) bulkDelete.mutate(chosen)
              }}
            >
              {t('media.delete')}
            </Button>
          </div>
        </div>
      )}

      {!q && (folders.data?.length ?? 0) > 0 && (
        <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {folders.data!.map((f) => (
            <li key={f.id} className="flex items-center gap-2 rounded-lg border border-border bg-surface p-3 shadow-sm hover:border-primary/40">
              <button
                type="button"
                title={t('media.open', { name: f.name })}
                onClick={() => {
                  setPath([...path, { id: f.id, name: f.name }])
                  setPage(1)
                }}
                className="flex min-w-0 flex-1 items-center gap-3 text-left"
              >
                <Folder className="h-6 w-6 shrink-0 text-primary" aria-hidden />
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">{f.name}</span>
                  <span className="block text-xs text-muted">{t('media.files', { count: String(f.filesCount) })}</span>
                </span>
              </button>
              {!pick && canUpdate && (
                <EntityActionsMenu
                  label={t('cm.actions')}
                  actions={[
                    { label: t('media.renameFolder'), icon: <Pencil className="h-4 w-4" />, onSelect: () => setRenaming(f) },
                    { label: t('media.moveFolderAction'), icon: <FolderInput className="h-4 w-4" />, onSelect: () => setMovingFolder(f) },
                    {
                      label: t('media.deleteFolder'),
                      icon: <Trash2 className="h-4 w-4" />,
                      destructive: true,
                      onSelect: async () => {
                        if (await confirm({ title: t('media.deleteFolder'), message: t('media.deleteFolderConfirm', { name: f.name }), confirmLabel: t('media.deleteFolder'), destructive: true })) removeFolder.mutate(f)
                      },
                    },
                  ]}
                />
              )}
            </li>
          ))}
        </ul>
      )}

      {list.length === 0 && !files.isPending ? (
        <Card className="text-center text-sm text-muted">{t('media.empty')}</Card>
      ) : (
        <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
          {list.map((f) => {
            const on = selectedIds.has(f.id)
            return (
              <li key={f.id}>
                <button
                  type="button"
                  title={pick || selecting ? t('media.selectFile', { name: f.name }) : t('media.open', { name: f.name })}
                  aria-pressed={pick || selecting ? on : undefined}
                  onClick={() => click(f)}
                  className={cn('group relative block w-full overflow-hidden rounded-lg border bg-surface text-left shadow-sm', on ? 'border-primary ring-2 ring-primary' : 'border-border hover:border-primary/40')}
                >
                  <span className="block aspect-square bg-background">
                    <Thumb file={f} />
                  </span>
                  <span className="block truncate px-2 py-1.5 text-xs">{f.name}</span>
                  {on && (
                    <span className="absolute top-2 right-2 rounded-full bg-primary p-1 text-primary-foreground">
                      <Check className="h-3 w-3" aria-hidden />
                    </span>
                  )}
                </button>
              </li>
            )
          })}
        </ul>
      )}
      {pagination && pagination.pageCount > 1 && (
        <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-end">
          <span className="text-center text-sm text-muted">{t('cm.page', { page: String(page), pages: String(pagination.pageCount) })}</span>
          <Button variant="outline" title={t('cm.previous')} disabled={page <= 1} onClick={() => setPage(page - 1)} className="w-full md:w-auto">
            {t('cm.previous')}
          </Button>
          <Button variant="outline" title={t('cm.next')} disabled={page >= pagination.pageCount} onClick={() => setPage(page + 1)} className="w-full md:w-auto">
            {t('cm.next')}
          </Button>
        </div>
      )}
      {creating && <NewFolder parent={folder} onClose={() => setCreating(false)} />}
      {renaming && <NewFolder parent={folder} folder={renaming} onClose={() => setRenaming(null)} />}
      {importing && <UrlDialog folder={folder} onClose={() => setImporting(false)} />}
      {movingFolder && <MoveDialog ids={[]} folder={movingFolder} onClose={() => setMovingFolder(null)} />}
      {moving && (
        <MoveDialog
          ids={chosen}
          onClose={() => {
            setMoving(false)
            setChosen([])
          }}
        />
      )}
    </div>
  )
}
