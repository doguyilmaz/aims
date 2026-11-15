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
