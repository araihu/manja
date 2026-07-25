// Package core is a temporary compatibility bridge while self-hosted adapters
// migrate to the public domain package. New behavior belongs in domain.
package core

import "github.com/araihu/manja/domain"

type Project = domain.Project
type ProjectSEO = domain.ProjectSEO
type ThemeSettings = domain.ThemeSettings
type Source = domain.Source
type Credential = domain.Credential
type Publication = domain.Publication
type Actor = domain.Actor
type SpecFile = domain.SpecFile
type Revision = domain.ContractRevision
type ContractRevision = domain.ContractRevision
type RevisionCandidate = domain.RevisionCandidate
type ReviewInputLocator = domain.ReviewInputLocator
type SpecIndex = domain.SpecIndex
type DocsBranding = domain.DocsBranding
type DocsBrandingLogo = domain.DocsBrandingLogo
type SpecDownload = domain.SpecDownload
type SpecOverview = domain.SpecOverview
type SpecContact = domain.SpecContact
type SpecLicense = domain.SpecLicense
type SpecServer = domain.SpecServer
type SpecServerVariable = domain.SpecServerVariable
type Operation = domain.Operation
type OperationParameter = domain.OperationParameter
type OperationRequestBody = domain.OperationRequestBody
type OperationResponse = domain.OperationResponse
type OperationMediaType = domain.OperationMediaType
type OperationSecurity = domain.OperationSecurity
type RequestSnippet = domain.RequestSnippet
type SchemaSummary = domain.SchemaSummary
type SchemaProperty = domain.SchemaProperty
type Schema = domain.Schema
type SchemaExample = domain.SchemaExample
type SearchDocument = domain.SearchDocument
type PublicRoute = domain.PublicRoute
type ContractSnapshot = domain.ContractSnapshot
type ContractOperation = domain.ContractOperation
type ContractParameter = domain.ContractParameter
type SpecDiff = domain.SpecDiff
type SpecChange = domain.SpecChange
type RuleLevel = domain.RuleLevel
type PolicyException = domain.PolicyException
type PolicyLayer = domain.PolicyLayer
type EffectivePolicy = domain.EffectivePolicy
type FindingDecision = domain.FindingDecision
type PolicyExceptionDisposition = domain.PolicyExceptionDisposition
type PolicyResult = domain.PolicyResult
type SnapshotRef = domain.SnapshotRef
type ComparisonReport = domain.ComparisonReport
type EffectivePolicyProjection = domain.EffectivePolicyProjection
type PolicyLayerProjection = domain.PolicyLayerProjection
type PolicyRuleProjection = domain.PolicyRuleProjection
type ReviewReport = domain.ReviewReport
type ReviewRequest = domain.ReviewRequest
type ReleaseMode = domain.ReleaseMode
type ReleaseTrack = domain.ReleaseTrack
type ContractReview = domain.ContractReview
type AuditEvent = domain.AuditEvent
type OutboxMessage = domain.OutboxMessage
type SyncRecord = domain.SyncRecord

const (
	SpecChangeAdditive                = domain.SpecChangeAdditive
	SpecChangeBreaking                = domain.SpecChangeBreaking
	RuleOperationRemoved              = domain.RuleOperationRemoved
	RuleOperationAdded                = domain.RuleOperationAdded
	RuleRequiredParameterAdded        = domain.RuleRequiredParameterAdded
	RuleParameterBecameRequired       = domain.RuleParameterBecameRequired
	RuleRequestBodyBecameRequired     = domain.RuleRequestBodyBecameRequired
	RuleResponseStatusRemoved         = domain.RuleResponseStatusRemoved
	RuleResponseStatusAdded           = domain.RuleResponseStatusAdded
	RuleSchemaRemoved                 = domain.RuleSchemaRemoved
	RuleSchemaAdded                   = domain.RuleSchemaAdded
	RuleLevelAllow                    = domain.RuleLevelAllow
	RuleLevelWarn                     = domain.RuleLevelWarn
	RuleLevelFail                     = domain.RuleLevelFail
	PolicySourceRepository            = domain.PolicySourceRepository
	PolicySourceServer                = domain.PolicySourceServer
	VerdictPass                       = domain.VerdictPass
	VerdictFail                       = domain.VerdictFail
	ExceptionDispositionApplied       = domain.ExceptionDispositionApplied
	ExceptionDispositionExpired       = domain.ExceptionDispositionExpired
	ExceptionDispositionNotApplicable = domain.ExceptionDispositionNotApplicable
	ReviewSchemaVersion               = domain.ReviewSchemaVersion
	ComparisonPullRequest             = domain.ComparisonPullRequest
	ComparisonReleaseImpact           = domain.ComparisonReleaseImpact
	ReleaseModePinned                 = domain.ReleaseModePinned
	ReleaseModeFollowing              = domain.ReleaseModeFollowing
	SyncResultSuccess                 = domain.SyncResultSuccess
	SyncResultFailure                 = domain.SyncResultFailure
)

var NewContractSnapshot = domain.NewContractSnapshot
var DiffSpecIndexes = domain.DiffSpecIndexes
var DiffContractSnapshots = domain.DiffContractSnapshots
var MergePolicy = domain.MergePolicy
var EvaluateFindings = domain.EvaluateFindings
var EvaluateReview = domain.EvaluateReview
var CanonicalReviewJSON = domain.CanonicalReviewJSON
var ConsiderReleaseRevision = domain.ConsiderReleaseRevision
var PromoteReleaseRevision = domain.PromoteReleaseRevision
