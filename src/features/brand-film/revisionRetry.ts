import { CreativeApiError } from '../../data/api'

type RecoveredMutation<T> = {
  value: T
  recovered: boolean
}

const isCreativeRevisionRace = (cause: unknown) => cause instanceof CreativeApiError && (
  cause.status === 412
  || cause.code === 'CREATIVE_VERSION_CONFLICT'
  || (cause.status === 409 && cause.code === 'INVALID_STATE')
)

export async function recoverCompletedBrandFilmMutation<T>(
  mutate: () => Promise<T>,
  refresh: () => Promise<T>,
  completed: (latest: T) => boolean,
): Promise<RecoveredMutation<T>> {
  try {
    return { value: await mutate(), recovered: false }
  } catch (cause) {
    if (!isCreativeRevisionRace(cause)) throw cause
    const latest = await refresh()
    if (!completed(latest)) throw cause
    return { value: latest, recovered: true }
  }
}

export async function runWithLatestCreativeRevision<T>(
  initialRevision: number,
  mutate: (expectedRevision: number) => Promise<T>,
  refreshRevision: () => Promise<number>,
): Promise<T> {
  try {
    return await mutate(initialRevision)
  } catch (cause) {
    if (!(cause instanceof CreativeApiError) || cause.status !== 412) throw cause
    const latestRevision = await refreshRevision()
    return mutate(latestRevision)
  }
}
