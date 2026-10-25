import { useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, CheckSquare, ChevronRight, File, Folder, FolderInput, FolderPlus, Link2, Pencil, Trash2, Upload, X } from 'lucide-react'
import { toast } from 'sonner'
import { DialogActions } from '@/components/DialogActions'
import { Pager } from '@/components/Pager'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { EntityActionsMenu } from '@/components/ui/entity-actions-menu'
import { Table, Td, Th, Tr } from '@/components/ui/table'
import { Tabs } from '@/components/ui/tabs'
import { SelectField, TextField } from '@/components/ui/field'
import { api, ApiError, requestWithMeta, upload } from '@/lib/api'
import { cn } from '@/lib/cn'
import { accepts } from '@/lib/media'
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
      footer={<DialogActions onCancel={onClose} onConfirm={() => create.mutate()} label={folder ? t('common.save') : t('media.create')} icon={<FolderPlus className="h-4 w-4" />} loading={create.isPending} />}
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
      footer={<DialogActions onCancel={onClose} onConfirm={() => send.mutate()} label={t('media.import')} icon={<Link2 className="h-4 w-4" />} loading={send.isPending} disabled={!urls.trim()} />}
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
      footer={<DialogActions onCancel={onClose} onConfirm={() => move.mutate()} label={t('media.move')} icon={<FolderInput className="h-4 w-4" />} loading={move.isPending} />}
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
  const { t, language } = useI18n()
  const { permissions } = useAuth()
  const queryClient = useQueryClient()
  const [path, setPath] = useState<Crumb[]>([{ id: null, name: t('media.root') }])
  const [q, setQ] = useState('')
  // allowedTypes are plural in Strapi schemas (images, videos, audios, files).
  const initialType = pick?.allowedTypes?.length === 1 ? ({ images: 'image', videos: 'video', audios: 'audio', files: 'file' }[pick.allowedTypes[0]!] ?? '') : ''
  const [type, setType] = useState(initialType)
  // A picker for a field with allowedTypes offers only those kinds.
  const kinds = (['image', 'video', 'audio', 'file'] as const).filter((k) => !pick?.allowedTypes?.length || pick.allowedTypes.includes(`${k}s`))
  const allowed = (f: MediaFile) => accepts(pick?.allowedTypes, f.mime)
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [view, setViewState] = useState<'grid' | 'list'>(() => {
    try {
      return localStorage.getItem('grapi.media.view') === 'list' ? 'list' : 'grid'
    } catch {
      return 'grid'
    }
  })
  const setView = (v: 'grid' | 'list') => {
    setViewState(v)
    try {
      localStorage.setItem('grapi.media.view', v)
    } catch {
      // Private mode: the choice lasts for the page.
    }
  }
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
  const config = useQuery({ queryKey: ['media', 'configuration'], queryFn: () => api.get<{ pageSize: number; sort: string }>('/upload/configuration'), staleTime: 60_000 })
  const [sort, setSort] = useState<string | null>(null)
  const sortBy = sort ?? config.data?.sort ?? 'createdAt:DESC'
  const params = new URLSearchParams({ folder: folderParam, page: String(page), pageSize: String(config.data?.pageSize ?? 10), sort: sortBy })
  if (q) params.set('_q', q)
  if (type) params.set('type', type)
  if (from) params.set('from', new Date(from).toISOString())
  if (to) params.set('to', new Date(to).toISOString())
  const files = useQuery({
    queryKey: ['media', 'files', params.toString()],
    queryFn: () => requestWithMeta<MediaFile[], { pagination: Pagination }>(`/upload/files?${params}`),
    placeholderData: (prev) => prev,
  })
  const folders = useQuery({ queryKey: ['media', 'folders', folderParam], queryFn: () => api.get<MediaFolder[]>(`/upload/folders?parent=${folderParam}`), enabled: !q })
  const canUpload = can(permissions, 'plugin::upload.assets.create')

  const [progress, setProgress] = useState<number | null>(null)
  const abort = useRef<AbortController | null>(null)
  const send = useMutation({
    mutationFn: (list: File[]) => {
      const form = new FormData()
      for (const f of list) form.append('files', f)
      abort.current = new AbortController()
      setProgress(0)
      return upload<MediaFile[]>(`/upload?folder=${folderParam}`, form, { onProgress: setProgress, signal: abort.current.signal })
    },
    onSuccess: (res) => {
      toast.success(t('media.uploaded', { count: String(res.length) }))
      void queryClient.invalidateQueries({ queryKey: ['media'] })
      // In a picker, what was just uploaded is what the user wants to pick.
      if (pick) {
        const fresh = res.filter(allowed)
        if (fresh.length) pick.onChange(pick.multiple ? [...pick.selected, ...fresh] : fresh.slice(0, 1))
      }
    },
    onError: (err) => (err instanceof DOMException && err.name === 'AbortError' ? toast.info(t('media.uploadCancelled')) : showError(err, undefined, t('common.error'))),
    onSettled: () => {
      setProgress(null)
      abort.current = null
    },
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
    if (!allowed(f)) return toast.error(t('media.notAllowedType'))
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
      {(canUpload || (!pick && canUpdate)) && (
        <div className="flex flex-col gap-2 md:flex-row md:flex-wrap *:whitespace-nowrap">
          {canUpload && !pick && (
            <Button title={t('media.upload')} icon={<Upload className="h-4 w-4" />} loading={send.isPending} onClick={() => input.current?.click()} className="w-full md:w-auto">
              {send.isPending ? t('media.uploading') : t('media.upload')}
            </Button>
          )}
          {canUpload && !pick && (
            <Button variant="outline" title={t('media.fromUrl')} icon={<Link2 className="h-4 w-4" />} onClick={() => setImporting(true)} className="w-full md:w-auto">
              {t('media.fromUrl')}
            </Button>
          )}
          {canUpload && (
            <Button variant="outline" title={t('media.newFolder')} icon={<FolderPlus className="h-4 w-4" />} onClick={() => setCreating(true)} className="w-full md:w-auto">
              {t('media.newFolder')}
            </Button>
          )}
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
          {canUpload && <input ref={input} type="file" multiple hidden onChange={(e) => onFiles(e.target.files)} />}
        </div>
      )}
      <div className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-3 shadow-sm">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,2fr)_repeat(2,minmax(0,1fr))]">
          <input
            type="search"
            value={q}
            placeholder={t('media.search')}
            aria-label={t('media.search')}
            onChange={(e) => {
              setQ(e.target.value)
              setPage(1)
            }}
            className="w-full self-end rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-primary focus:outline-none"
          />
          <SelectField
            label={t('media.sort')}
            value={sortBy}
            onChange={(e) => {
              setSort(e.target.value)
              setPage(1)
            }}
          >
            {(['createdAt:DESC', 'createdAt:ASC', 'name:ASC', 'name:DESC', 'updatedAt:DESC', 'updatedAt:ASC'] as const).map((k) => (
              <option key={k} value={k}>
                {t(`media.sort.${k}`)}
              </option>
            ))}
          </SelectField>
          <SelectField label={t('media.type')} value={type} onChange={(e) => setType(e.target.value)}>
            {kinds.length !== 1 && <option value="">{t('media.type.all')}</option>}
            {kinds.map((k) => (
              <option key={k} value={k}>
                {t(`media.type.${k}`)}
              </option>
            ))}
          </SelectField>
        </div>
        <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 md:w-[28rem]">
            <TextField
              label={t('media.from')}
              type="datetime-local"
              value={from}
              onChange={(e) => {
                setFrom(e.target.value)
                setPage(1)
              }}
            />
            <TextField
              label={t('media.to')}
              type="datetime-local"
              value={to}
              onChange={(e) => {
                setTo(e.target.value)
                setPage(1)
              }}
            />
          </div>
          <Tabs
            tabs={[
              { id: 'grid', label: t('media.viewGrid') },
              { id: 'list', label: t('media.viewList') },
            ]}
            value={view}
            onChange={(id) => setView(id as 'grid' | 'list')}
          />
        </div>
      </div>
      {progress !== null && (
        <div className="flex flex-col gap-2 rounded-lg border border-border bg-surface p-3 md:flex-row md:items-center">
          <div className="flex-1">
            <p className="mb-1 text-sm">{t('media.uploadProgress', { percent: String(Math.round(progress * 100)) })}</p>
            <div className="h-2 overflow-hidden rounded bg-background" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(progress * 100)}>
              <div className="h-full bg-primary transition-[width]" style={{ width: `${Math.round(progress * 100)}%` }} />
            </div>
          </div>
          <Button variant="destructive-outline" title={t('media.cancelUpload')} icon={<X className="h-4 w-4" />} onClick={() => abort.current?.abort()} className="w-full md:w-auto">
            {t('media.cancelUpload')}
          </Button>
        </div>
      )}
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
                        if (await confirm({ title: t('media.deleteFolder'), message: t('media.deleteFolderConfirm', { name: f.name, files: String(f.totalFiles ?? f.filesCount), folders: String(f.totalFolders ?? f.childrenCount) }), confirmLabel: t('media.deleteFolder'), destructive: true })) removeFolder.mutate(f)
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
        view === 'list' ? (
        <>
          <div className="hidden md:block">
            <Table>
              <thead>
                <tr>
                  <Th className="w-16">
                    <span className="sr-only">{t('media.preview')}</span>
                  </Th>
                  <Th>{t('media.name')}</Th>
                  <Th>{t('media.mime')}</Th>
                  <Th>{t('media.size')}</Th>
                  <Th>{t('media.createdAt')}</Th>
                </tr>
              </thead>
              <tbody>
                {list.map((f) => {
                  const on = selectedIds.has(f.id)
                  return (
                    <Tr key={f.id} className={cn('cursor-pointer', on && 'bg-primary/10')} onClick={() => click(f)} title={pick || selecting ? t('media.selectFile', { name: f.name }) : t('media.open', { name: f.name })}>
                      <Td>
                        <span className="relative block h-10 w-10 overflow-hidden rounded bg-background">
                          <Thumb file={f} />
                          {on && (
                            <span className="absolute inset-0 flex items-center justify-center bg-primary/60 text-primary-foreground">
                              <Check className="h-4 w-4" aria-hidden />
                            </span>
                          )}
                        </span>
                      </Td>
                      <Td className="max-w-xs truncate font-medium">{f.name}</Td>
                      <Td className="text-muted">{f.mime}</Td>
                      <Td className="text-muted">{f.size.toLocaleString(language)} KB</Td>
                      <Td className="text-muted">{new Date(f.createdAt).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' })}</Td>
                    </Tr>
                  )
                })}
              </tbody>
            </Table>
          </div>
          <ul className="flex flex-col gap-2 md:hidden">
            {list.map((f) => {
              const on = selectedIds.has(f.id)
              return (
                <li key={f.id}>
                  <button
                    type="button"
                    title={pick || selecting ? t('media.selectFile', { name: f.name }) : t('media.open', { name: f.name })}
                    aria-pressed={pick || selecting ? on : undefined}
                    onClick={() => click(f)}
                    className={cn('flex w-full items-center gap-3 rounded-lg border bg-surface p-2 text-left shadow-sm', on ? 'border-primary ring-2 ring-primary' : 'border-border')}
                  >
                    <span className="block h-12 w-12 shrink-0 overflow-hidden rounded bg-background">
                      <Thumb file={f} />
                    </span>
                    <span className="min-w-0">
                      <span className="block truncate text-sm font-medium">{f.name}</span>
                      <span className="block text-xs text-muted">
                        {f.mime} · {f.size.toLocaleString(language)} KB
                      </span>
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
        </>
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
        )
      )}
      {pagination && pagination.pageCount > 1 && <Pager page={page} pageCount={pagination.pageCount} onPage={setPage} className="md:justify-end" />}
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
