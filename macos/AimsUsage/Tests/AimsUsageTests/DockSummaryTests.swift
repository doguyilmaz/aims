import Foundation
import Testing
import UsageCore

@testable import AimsUsage

@Test @MainActor func dockUsesOnlyCurrentReadingsWithoutChangingStoredWindows() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-13T00:00:00Z","providers":[{"id":"codex","name":"Codex","installed":true,"rows":[
              {"id":"old","name":"Old","observedAt":"2026-11-12T00:00:00Z","windows":[{"label":"7d","pct":99}]},
              {"id":"fresh","name":"Fresh","observedAt":"2026-11-12T23:59:00Z","windows":[{"label":"passed","pct":100,"resetsAt":"2026-11-12T23:00:00Z"},{"label":"7d","pct":41,"resetsAt":"2026-11-14T00:00:00Z"}]}]}]}
            """#.utf8))
    let provider = snapshot.providers[0]
    let summary = DockSummary(provider: provider, now: snapshot.generatedAt)
    #expect(summary.percent == 41)
    #expect(summary.value == "41%")
    #expect(!summary.isCached)
    #expect(summary.currentPercent == 41)
    #expect(DockSummary(provider: provider, now: snapshot.generatedAt, globalError: "CLI failed").percent == nil)
    #expect(provider.rows[0].windows?.first?.pct == 99)
    #expect(provider.rows[1].windows?.first?.pct == 100)
}

@Test @MainActor func unknownOrFailedDockReadingsNeverBecomeZeroAndBlockingStaysExplicit() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-13T00:00:00Z","providers":[
              {"id":"unknown","name":"Unknown","installed":true,"rows":[{"id":"a","name":"A","observedAt":"2026-11-12T23:59:00Z","windows":[{"label":"budget","pct":0,"pctUnknown":true}]}]},
              {"id":"blocked","name":"Blocked","installed":true,"rows":[{"id":"a","name":"A","observedAt":"2026-11-12T23:59:00Z","windows":[{"label":"budget","pct":0,"pctUnknown":true,"limited":true}]}]},
              {"id":"error","name":"Error","installed":true,"error":"refresh failed","rows":[{"id":"a","name":"A","observedAt":"2026-11-12T23:59:00Z","windows":[{"label":"7d","pct":77}]}]}
            ]}
            """#.utf8))
    #expect(DockSummary(provider: snapshot.providers[0], now: snapshot.generatedAt).value == "—")
    let blocked = DockSummary(provider: snapshot.providers[1], now: snapshot.generatedAt)
    #expect(blocked.percent == nil)
    #expect(blocked.value == "Blocked")
    #expect(DockSummary(provider: snapshot.providers[2], now: snapshot.generatedAt).percent == nil)
}

@Test @MainActor func staleClaudeKeepsLastKnownUsageInsteadOfBecomingMissing() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-14T12:45:00Z","providers":[{"id":"claude","name":"Claude","installed":true,"rows":[
              {"id":"personal","name":"personal","observedAt":"2026-11-13T10:31:00Z","windows":[{"label":"5h","pct":100,"resetsAt":"2026-11-13T11:20:00Z"},{"label":"7d","pct":41,"resetsAt":"2026-11-16T17:00:00Z"}]},
              {"id":"work","name":"work","observedAt":"2026-11-14T10:19:00Z","windows":[{"label":"5h","pct":5,"resetsAt":"2026-11-14T14:10:00Z"},{"label":"7d","pct":80,"resetsAt":"2026-11-15T04:00:00Z"}]}]}]}
            """#.utf8))
    let summary = DockSummary(provider: snapshot.providers[0], now: snapshot.generatedAt)
    #expect(summary.percent == 80)
    #expect(summary.value == "80%")
    #expect(summary.isCached)
    #expect(summary.currentPercent == nil)
    #expect(MenuSummary.value(snapshot: snapshot, now: snapshot.generatedAt, globalError: nil) == nil)
    #expect(snapshot.providers[0].rows[0].windows?.first?.pct == 100)
}

@Test @MainActor func cachedZeroIsKnownButUnknownExpiredAndFutureReadingsStayUnavailable() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-14T12:45:00Z","providers":[
              {"id":"zero","name":"Zero","installed":true,"rows":[{"id":"a","name":"A","observedAt":"2026-11-13T10:00:00Z","windows":[{"label":"7d","pct":0,"resetsAt":"2026-11-15T00:00:00Z"}]}]},
              {"id":"unknown","name":"Unknown","installed":true,"rows":[{"id":"a","name":"A","observedAt":"2026-11-13T10:00:00Z","windows":[{"label":"7d","pct":0,"pctUnknown":true,"resetsAt":"2026-11-15T00:00:00Z"}]}]},
              {"id":"expired","name":"Expired","installed":true,"rows":[{"id":"a","name":"A","observedAt":"2026-11-13T10:00:00Z","windows":[{"label":"5h","pct":100,"resetsAt":"2026-11-14T12:45:00Z"}]}]},
              {"id":"future","name":"Future","installed":true,"rows":[{"id":"a","name":"A","observedAt":"2026-11-15T00:00:00Z","windows":[{"label":"7d","pct":99,"resetsAt":"2026-11-16T00:00:00Z"}]}]}
            ]}
            """#.utf8))
    let zero = DockSummary(provider: snapshot.providers[0], now: snapshot.generatedAt)
    #expect(zero.value == "0%")
    #expect(zero.isCached)
    for provider in snapshot.providers.dropFirst() {
        let summary = DockSummary(provider: provider, now: snapshot.generatedAt)
        #expect(summary.percent == nil)
        #expect(summary.value == "—")
        #expect(!summary.isCached)
    }
}
