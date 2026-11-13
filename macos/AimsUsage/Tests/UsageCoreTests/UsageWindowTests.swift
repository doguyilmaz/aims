import Foundation
import Testing

@testable import UsageCore

@Test func blockedUnknownRatioAndMissingQuotaNeverBecomeZeroUsage() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-12T00:00:00Z","providers":[{"id":"opencode","name":"OpenCode","installed":true,"rows":[
              {"id":"budget","name":"Budget","windows":[{"label":"weekly","pct":25},{"label":"budget","pct":0,"pctUnknown":true,"limited":true}]},
              {"id":"missing","name":"Missing","windows":[]}]}]}
            """#.utf8))
    let rows = snapshot.providers[0].rows
    let windows = try #require(rows[0].windows)
    #expect(windows.count == 2)
    #expect(windows[0].validPercent == 25)
    #expect(windows[1].validPercent == nil)
    #expect(windows[1].limited == true)
    #expect(rows[1].windows?.isEmpty == true)
}
