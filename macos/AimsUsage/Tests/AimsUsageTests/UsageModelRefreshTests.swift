import Foundation
import Testing

@testable import AimsUsage

@MainActor @Test func localChangesReloadWithoutAProviderRequestOrMaskingRefreshFailure() async {
    let fixture = UsageRefreshFixture()
    let model = UsageModel(
        configHomeOverride: "",
        commandRunner: { arguments in
            let live = arguments.contains("--refresh")
            fixture.calls.append(live)
            if live && fixture.failProvider { throw CocoaError(.fileReadUnknown) }
            return fixture.reading()
        })
    await model.reloadCached()
    #expect(model.snapshot?.providers.first?.rows.first?.windows?.first?.pct == 100)
    await model.refresh()
    let failure = model.error
    #expect(failure != nil)
    fixture.percent = 0
    await model.reloadCached()
    #expect(model.snapshot?.providers.first?.rows.first?.windows?.first?.pct == 0)
    #expect(model.error == failure)
    fixture.failProvider = false
    await model.refresh()
    #expect(model.error == nil)
    #expect(fixture.calls == [false, true, false, true])
}

@MainActor @Test func localReloadDoesNotOverlapAnInFlightProviderRefresh() async throws {
    let fixture = UsageRefreshFixture()
    let model = UsageModel(
        configHomeOverride: "",
        commandRunner: { arguments in
            let live = arguments.contains("--refresh")
            fixture.calls.append(live)
            fixture.ready?.resume()
            return try await withCheckedThrowingContinuation { fixture.response = $0 }
        })
    var request: Task<Void, Never>?
    await withCheckedContinuation { continuation in
        fixture.ready = continuation
        request = Task { await model.refresh() }
    }
    #expect(model.isRefreshing)
    await model.reloadCached()
    #expect(fixture.calls == [true])
    let pending = try #require(fixture.response)
    fixture.percent = 0
    pending.resume(returning: fixture.reading())
    await request?.value
    #expect(!model.isRefreshing)
    #expect(model.error == nil)
    #expect(model.snapshot?.providers.first?.rows.first?.windows?.first?.pct == 0)
}

@MainActor @Test func recoveredLocalReadClearsItsOwnFailureWithoutAProviderCall() async {
    let fixture = UsageRefreshFixture()
    fixture.failCache = true
    let model = UsageModel(
        configHomeOverride: "",
        commandRunner: { arguments in
            let live = arguments.contains("--refresh")
            fixture.calls.append(live)
            if fixture.failCache { throw CocoaError(.fileReadUnknown) }
            return fixture.reading()
        })
    await model.reloadCached()
    #expect(model.error != nil)
    fixture.failCache = false
    await model.reloadCached()
    #expect(model.error == nil)
    #expect(model.snapshot != nil)
    #expect(fixture.calls == [false, false])
}

@MainActor @Test func reminderEditsPreserveAFailedProviderRefreshUntilSuccessfulRetry() async {
    let fixture = UsageRefreshFixture()
    let model = UsageModel(
        configHomeOverride: "",
        commandRunner: { arguments in
            if arguments.contains("--refresh") && fixture.failProvider { throw CocoaError(.fileReadUnknown) }
            return fixture.reading()
        })
    await model.refresh()
    let failure = model.error
    #expect(failure != nil)
    let added = await model.addReminder(provider: "codex", row: "work", label: "Check reset", at: Date())
    #expect(added)
    #expect(model.error == failure)
    await model.removeReminder(id: "fixture-reminder")
    #expect(model.error == failure)
    #expect(model.snapshot != nil)
    fixture.failProvider = false
    await model.refresh()
    #expect(model.error == nil)
}
