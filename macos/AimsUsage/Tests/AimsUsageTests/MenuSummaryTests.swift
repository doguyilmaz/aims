import Foundation
import Testing
import UsageCore

@testable import AimsUsage

@Test @MainActor func menuDropsFailedAndExpiredReadingsWithoutWaitingForAnotherSnapshot() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-13T00:00:00Z","providers":[{"id":"codex","name":"Codex","installed":true,"rows":[{"id":"personal","name":"Personal","observedAt":"2026-11-13T00:00:00Z","windows":[{"label":"5h","pct":91,"resetsAt":"2026-11-13T00:01:00Z"},{"label":"7d","pct":36,"resetsAt":"2026-11-14T00:00:00Z"}]}]}]}
            """#.utf8))
    #expect(MenuSummary.value(snapshot: snapshot, now: snapshot.generatedAt, globalError: nil) == "91%")
    #expect(MenuSummary.value(snapshot: snapshot, now: snapshot.generatedAt, globalError: "CLI failed") == nil)
    #expect(
        MenuSummary.value(snapshot: snapshot, now: snapshot.generatedAt.addingTimeInterval(60), globalError: nil)
            == "36%")
    #expect(
        MenuSummary.value(snapshot: snapshot, now: snapshot.generatedAt.addingTimeInterval(7_201), globalError: nil)
            == nil)
    #expect(snapshot.providers[0].rows[0].windows?.first?.pct == 91)
}

@Test @MainActor func menuDoesNotPresentUnknownUninstalledOrFailedProviderQuotasAsCurrent() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-13T00:00:00Z","providers":[
              {"id":"unknown","name":"Unknown","installed":true,"rows":[{"id":"a","name":"A","observedAt":"2026-11-13T00:00:00Z","windows":[{"label":"budget","pct":0,"pctUnknown":true}]}]},
              {"id":"removed","name":"Removed","installed":false,"rows":[{"id":"a","name":"A","observedAt":"2026-11-13T00:00:00Z","windows":[{"label":"7d","pct":99}]}]},
              {"id":"failed","name":"Failed","installed":true,"error":"refresh failed","rows":[{"id":"a","name":"A","observedAt":"2026-11-13T00:00:00Z","windows":[{"label":"7d","pct":77}]}]}
            ]}
            """#.utf8))
    #expect(MenuSummary.value(snapshot: snapshot, now: snapshot.generatedAt, globalError: nil) == nil)
    #expect(MenuSummary.value(snapshot: nil, now: snapshot.generatedAt, globalError: nil) == nil)
}
