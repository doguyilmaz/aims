import Foundation
import Testing
import UsageCore

@testable import AimsUsage

@Test @MainActor func dockWindowFocusSelectsOnlyTheRequestedAllowanceAcrossAccounts() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-14T12:00:00Z","providers":[{"id":"codex","name":"Codex","installed":true,"rows":[
              {"id":"personal","name":"Personal","observedAt":"2026-11-14T12:00:00Z","windows":[{"label":"5h","pct":30},{"label":"7d","pct":90},{"label":"month","pct":37},{"label":"budget","pct":16}]},
              {"id":"work","name":"Work","observedAt":"2026-11-14T12:00:00Z","windows":[{"label":"5h","pct":10},{"label":"weekly","pct":40}]}]}]}
            """#.utf8))
    let provider = snapshot.providers[0]
    for (focus, used) in [
        (DockWindowFocus.automatic, 90.0), (.window("5h"), 30), (.window("7d"), 90),
        (.window("month"), 37), (.window("budget"), 16),
    ] {
        let summary = DockSummary(provider: provider, now: snapshot.generatedAt, focus: focus, valueMode: .remaining)
        #expect(summary.percent == used)
        #expect(summary.displayPercent == 100 - used)
        #expect(summary.currentPercent == used)
        #expect(!summary.isCached)
    }
    #expect(DockSummary(provider: provider, now: snapshot.generatedAt, focus: .window("5h")).value == "30%")
    #expect(
        DockSummary(provider: provider, now: snapshot.generatedAt, focus: .window("5h"), valueMode: .remaining).value
            == "70%")
    let choices = DockWindowFocus.options(for: provider)
    #expect(choices == [.automatic, .window("5h"), .window("7d"), .window("month"), .window("budget")])
    #expect(provider.rows[0].windows?.count == 4)
}

@Test @MainActor func chosenWindowKeepsCachedDataAndNeverSubstitutesAnUnrelatedWindow() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-14T12:00:00Z","providers":[{"id":"claude","name":"Claude","installed":true,"rows":[
              {"id":"old","name":"Old","observedAt":"2026-11-14T09:00:00Z","windows":[{"label":"5h","pct":20}]},
              {"id":"fresh","name":"Fresh","observedAt":"2026-11-14T12:00:00Z","windows":[{"label":"7d","pct":90}]}]}]}
            """#.utf8))
    let provider = snapshot.providers[0]
    let cached = DockSummary(provider: provider, now: snapshot.generatedAt, focus: .window("5h"), valueMode: .remaining)
    #expect(cached.value == "80%")
    #expect(cached.isCached)
    #expect(cached.currentPercent == nil)
    let missing = DockSummary(
        provider: provider, now: snapshot.generatedAt, focus: .window("month"), valueMode: .remaining)
    #expect(missing.value == "—")
    #expect(missing.displayPercent == nil)
    #expect(missing.explanation.contains("Monthly"))
    #expect(DockWindowFocus.options(for: provider, including: .window("month")).contains(.window("month")))
}

@Test @MainActor func remainingModeDoesNotInventAllowanceForUnknownOrBlockedReadings() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-14T12:00:00Z","providers":[
              {"id":"unknown","name":"Unknown","installed":true,"rows":[{"id":"a","name":"A","observedAt":"2026-11-14T12:00:00Z","windows":[{"label":"7d","pct":0,"pctUnknown":true}]}]},
              {"id":"blocked","name":"Blocked","installed":true,"rows":[{"id":"a","name":"A","observedAt":"2026-11-14T12:00:00Z","windows":[{"label":"7d","pct":0,"pctUnknown":true,"limited":true}]}]},
              {"id":"over","name":"Over","installed":true,"rows":[{"id":"a","name":"A","observedAt":"2026-11-14T12:00:00Z","windows":[{"label":"7d","pct":120}]}]}
            ]}
            """#.utf8))
    #expect(DockSummary(provider: snapshot.providers[0], now: snapshot.generatedAt, valueMode: .remaining).value == "—")
    #expect(
        DockSummary(provider: snapshot.providers[1], now: snapshot.generatedAt, valueMode: .remaining).value
            == "Blocked")
    #expect(
        DockSummary(provider: snapshot.providers[2], now: snapshot.generatedAt, valueMode: .remaining).value == "0%")
    for focus in [DockWindowFocus.automatic, .window("5h"), .window("7d"), .window("custom limit")] {
        #expect(DockWindowFocus(savedValue: focus.id) == focus)
    }
    #expect(DockWindowFocus(savedValue: "window: ") == .automatic)
}
