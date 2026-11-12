import Foundation
import Testing

@testable import UsageCore

@Test func displayKeepsProfilesAndFutureProvidersWithOpenCodeLast() throws {
    let ids = ["opencode", "future", "agy", "grok", "codex", "kiro", "claude"]
    let providers = ids.map { #"{"id":""# + $0 + #"","name":""# + $0 + #"","installed":true,"rows":[]}"# }
    let data = Data(
        (#"{"schemaVersion":1,"generatedAt":"2026-11-12T00:00:00Z","providers":["#
            + providers.joined(separator: ",") + "]}").utf8)
    let snapshot = try UsageSnapshot.decode(data)
    #expect(snapshot.displayProviders.map(\.id) == ["claude", "codex", "kiro", "grok", "agy", "future", "opencode"])
    #expect(snapshot.providers.map(\.id) == ids)
}
