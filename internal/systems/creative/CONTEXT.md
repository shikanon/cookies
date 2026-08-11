# Creative bounded context

## Ubiquitous language

- **CreativeIntake**: Creative owns the normalized, user-confirmed source for a
  planned creative output. It can be incomplete, but it is never a Strategy
  object.
- **CreativeTask**: A named production work created from a ready Intake. It owns
  the selected channel, production state, content drafts and production
  lineage. `display_name` is its user-facing identity in work selectors;
  Draft revisions are editing history inside the task, not separate works.
- **ImageTextDraft**: The editable working revision for one image-and-text
  task. It contains the post copy and planned image sequence; it is not a
  media asset or a cross-system hand-off.
- **CreativeVersion**: An immutable Creative-owned snapshot frozen from one
  Draft revision. Checks, review, approval, delivery and later systems refer
  to this stable identity rather than to an editable Draft or a Provider job.
- **CreativeCheck**: The recorded result of evaluating a frozen CreativeVersion
  against the agreed image group, mandatory elements and prohibited claims.
  A failed check is evidence, not an edit to the frozen version.
- **CreativePackage**: The delivery-safe Creative hand-off built only from an
  approved CreativeVersion. It contains frozen copy and `AssetVersionRef`s;
  Delivery and Insights never receive a mutable CreativeTask.
- **ProductionJob**: A reference from a CreativeTask to a Provider job. It
  records production lineage and does not become the task's business state.
- **CreativeDirection**: The user-selected expression of a message, including
  concept, tone and visual keywords. It can refine an upstream recommendation
  without changing that upstream object.
- **CreativeSourceVersion**: The immutable upstream version deliberately
  selected for one creative effort. It may be a confirmed Brief, an approved
  StrategyPackage or a development fixture; later upstream versions never
  alter an existing CreativeIntake.
- **VideoTemplateRecipe**: Creative's versioned production grammar for a video
  pattern. It defines required facts and assets, motion phases and preservation
  rules, but it is not the final model prompt.
- **PromptPackage**: An immutable, traceable compilation of one CreativeIntake
  and one VideoTemplateRecipe into structured directions and the exact model
  prompt shown for approval.
- **GenerationSpec**: The frozen, approved combination of a PromptPackage,
  conditioning assets and media settings used to request model production.
- **Candidate**: One model-produced output under evaluation. Provider success
  makes a Candidate available; it does not make the Candidate approved.
- **Ready Intake**: An Intake with a channel, objective, audience and core
  message. Only a ready Intake may create a CreativeTask.
- **AIAdWorkspace**: The Creative-owned aggregate for one AI-native performance
  video effort. It points to the current Requirement, Script, Storyboard and
  Production revisions without making Provider jobs its business state.
- **ProductSnapshot**: The immutable commerce-source facts captured for one
  AIAdWorkspace. Later changes to the external product page do not mutate it.
- **AdScriptRevision**: One complete, ordered marketing script derived from a
  confirmed Requirement revision. It is not a collection of candidates.
- **StoryboardRevision**: The immutable material plan and ordered Shot timeline
  derived from one confirmed AdScriptRevision.
- **Storyboard Asset Role**: The explicit identity of a storyboard reference:
  product, person, scene, composition, audio or brand element. A role is not
  inferred from a filename or prompt.
- **Product Identity Asset**: A real `AssetVersionRef` imported from the
  confirmed product source or selected project assets. AI-generated media may
  support people, scenes and composition, but can never replace this identity.
- **Storyboard Plan**: Recoverable operation state produced before missing
  reference images finish Provider and Assets ingestion. Only an all-ready
  plan can become an editable StoryboardRevision.
- **ProductionPlan**: The frozen mapping from a confirmed StoryboardRevision to
  executable image, video, speech and timeline work.
- **GenerationUnit**: One provider-executable slice of a ProductionPlan. Brand
  Film keeps exactly one GenerationUnit per Shot; other performance modes may
  define a different mapping.
- **GenerationAttempt**: One concrete Provider execution for a GenerationUnit.
  Retrying adds an Attempt and never overwrites a prior result.
- **ChannelCreativeProfile**: A versioned Creative-owned set of channel writing,
  pacing, subtitle and CTA rules compiled into a PromptPackage.
- **Audio Blueprint**: A versioned, explainable suggestion that maps a confirmed
  FilmPlan to narration, music and sound-effect cues. It is not rendered media.
- **AudioMixVariant**: One tone or language treatment over the same locked visual
  preview. It owns an immutable sequence of AudioMixVersions.
- **AudioMixVersion**: One authoritative editable audio timeline snapshot. An
  edit appends a revision; it never mutates an earlier mix.
- **AudioMixRenderJob**: A durable asynchronous request binding one immutable
  AudioMixVersion and renderer version to one preview or final AssetVersion.
- **AudioMixCompiler**: Creative's translation from the authoritative mix
  revision to a renderer-neutral timeline request.
- **AudioMixRenderer**: Media's deep module for trim, delay, fades, gain,
  voice-triggered music ducking, limiting and loudness normalization without
  regenerating the visual.
- **Logical Voice Alias**: A stable Creative-owned voice identity such as
  `cookies.voice.brand.warm_female`. Provider-specific voice IDs stay behind
  the encrypted speech route and can change without rewriting saved mixes.
- **SpeechCapability**: A project-scoped, live Provider probe result. It states
  whether real speech generation is available and which logical voices may be
  selected; it never exposes credentials.
- **AudioGenerationAttempt**: One immutable attempt to replace a planned audio
  clip with generated media. A failed attempt records a classified Provider
  error while the prior Fixture or user asset remains authoritative.
- **AudioDirectorDecision**: One explainable planner recommendation binding a
  narration placement, music-ducking rule or beat snap to a concrete mix
  target. It is a suggestion until represented by an immutable MixVersion.
- **BrandPronunciation**: A provider-neutral mapping from approved brand text
  to its intended spoken form. It belongs to the Audio Blueprint and does not
  expose a provider voice identifier.
- **AudioSemanticCheck**: A deterministic comparison between what narration
  claims and what the corresponding Shot shows, with evidence and a proposed
  repair rather than an automatic script mutation.
