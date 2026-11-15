import Foundation
import Testing

@testable import UsageCore

@Test func preservesAccountGroupsAndResetInventory() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-12T00:00:00.123456Z","providers":[
              {"id":"codex","name":"Codex","installed":true,"rows":[
                {"id":"personal","name":"personal","windows":[{"label":"5h","pct":91,"resetsAt":"2026-11-12T01:00:00Z"}],"bankedResets":{"available":2,"credits":null},"observedAt":"2026-11-11T23:00:00Z"},
                {"id":"work","name":"work","windows":[],"bankedResets":null}]},
              {"id":"agy","name":"Antigravity","installed":true,"rows":[
                {"id":"argon","name":"Gemini 4 Argon","windows":[]},
                {"id":"carbon","name":"Gemini 4 Carbon","windows":[]}]}]}
            """#.utf8))
    #expect(snapshot.providers.count == 2)
    #expect(snapshot.providers[0].rows[0].bankedResets?.available == 2)
    #expect(snapshot.providers[0].rows[0].bankedResets?.credits == nil)
    #expect(snapshot.providers[0].rows[1].bankedResets == nil)
    #expect(snapshot.providers[1].rows.map(\.id) == ["argon", "carbon"])
    #expect(snapshot.generatedAt.timeIntervalSince1970 > 0)
}

@Test func pastResetRequiresRefreshWithoutChangingUsage() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-12T00:00:00Z","providers":[{"id":"codex","name":"Codex","installed":true,"rows":[{"id":"work","name":"work","windows":[{"label":"7d","pct":100,"resetsAt":"2026-11-11T00:00:00Z"}],"observedAt":"2026-11-10T00:00:00Z"}]}]}
            """#.utf8))
    let row = try #require(snapshot.providers.first?.rows.first)
    let window = try #require(row.windows?.first)
    #expect(window.needsRefresh(at: snapshot.generatedAt))
    #expect(window.validPercent == 100)
    #expect(row.isStale(at: snapshot.generatedAt))
    #expect(
        ResetPresentation.countdown(to: try #require(window.resetsAt), now: snapshot.generatedAt) == "Refresh needed")
}

@Test func rejectsUnsupportedSchema() {
    #expect(throws: UsageError.self) {
        try UsageSnapshot.decode(Data(#"{"schemaVersion":2,"generatedAt":"2026-11-12T00:00:00Z","providers":[]}"#.utf8))
    }
}
