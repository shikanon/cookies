export type SaveCoordinatorState<Version> = {
  status: 'clean' | 'dirty' | 'saving' | 'error'
  version: Version
  error?: unknown
}

type Deferred<Version> = {
  resolve: (version: Version) => void
  reject: (cause: unknown) => void
}

type PendingSave<Document, Version> = {
  document: Document
  baseVersion: Version
  deferred: Deferred<Version>[]
}

export class SaveCoordinator<Document, Version> {
  state: SaveCoordinatorState<Version>
  private active = false
  private pending: PendingSave<Document, Version> | null = null

  constructor(private readonly persist: (document: Document, baseVersion: Version) => Promise<Version>, initialVersion: Version = 0 as Version) {
    this.state = { status: 'clean', version: initialVersion }
  }

  submit(document: Document, baseVersion: Version): Promise<Version> {
    this.state = { ...this.state, status: this.active ? 'dirty' : 'saving', error: undefined }
    return new Promise<Version>((resolve, reject) => {
      if (this.active) {
        if (this.pending) {
          this.pending.document = document
          this.pending.deferred.push({ resolve, reject })
        } else {
          this.pending = { document, baseVersion, deferred: [{ resolve, reject }] }
        }
        return
      }
      this.active = true
      void this.run({ document, baseVersion, deferred: [{ resolve, reject }] })
    })
  }

  private async run(save: PendingSave<Document, Version>): Promise<void> {
    try {
      const version = await this.persist(save.document, save.baseVersion)
      for (const deferred of save.deferred) deferred.resolve(version)
      this.state = { status: this.pending ? 'saving' : 'clean', version }
      const next = this.pending
      this.pending = null
      if (next) {
        await this.run({ ...next, baseVersion: version })
        return
      }
      this.active = false
    } catch (cause) {
      for (const deferred of save.deferred) deferred.reject(cause)
      if (this.pending) {
        for (const deferred of this.pending.deferred) deferred.reject(cause)
      }
      this.pending = null
      this.active = false
      this.state = { ...this.state, status: 'error', error: cause }
    }
  }
}
