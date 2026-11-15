import Foundation
import Testing
import UsageCore

@testable import AimsUsage

@Test @MainActor func dockAccountSelectionComposesWithWindowAndValuePreferences() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-15T00:00:00Z","providers":[{"id":"codex","name":"Codex","installed":true,"rows":[
              {"id":"personal","name":"personal","observedAt":"2026-11-15T00:00:00Z","windows":[{"label":"5h","pct":30},{"label":"7d","pct":90}]},
              {"id":"work","name":"work","observedAt":"2026-11-15T00:00:00Z","windows":[{"label":"5h","pct":10},{"label":"7d","pct":40}]}]}]}
            """#.utf8))
    let provider = snapshot.providers[0]
    #expect(DockSummary(provider: provider, now: snapshot.generatedAt).value == "90%")
    let work = DockSummary(provider: provider, now: snapshot.generatedAt, account: .account("work"))
    #expect(work.value == "40%")
    #expect(work.explanation.contains("Work"))
    #expect(!work.explanation.contains("across accounts"))
    #expect(
        DockSummary(
            provider: provider, now: snapshot.generatedAt, focus: .window("5h"),
            valueMode: .remaining, account: .account("work")
        ).value == "90%")
    #expect(
        DockSummary(
            provider: provider, now: snapshot.generatedAt, focus: .window("5h"),
            account: .account("personal")
        ).value == "30%")
    #expect(MenuSummary.value(snapshot: snapshot, now: snapshot.generatedAt, globalError: nil) == "90%")
    #expect(provider.rows.count == 2 && provider.rows[0].windows?.count == 2)
}

@Test @MainActor func chosenAccountNeverFallsBackToAnotherAccountsFreshOrFailedData() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-15T00:00:00Z","providers":[{"id":"claude","name":"Claude","installed":true,"rows":[
              {"id":"personal","name":"Personal","observedAt":"2026-11-14T20:00:00Z","windows":[{"label":"7d","pct":71,"resetsAt":"2026-11-16T00:00:00Z"}]},
              {"id":"work","name":"Work","observedAt":"2026-11-15T00:00:00Z","windows":[{"label":"7d","pct":12}]},
              {"id":"failed","name":"Failed","error":"Login unavailable","observedAt":"2026-11-15T00:00:00Z","windows":[{"label":"7d","pct":99}]}]}]}
            """#.utf8))
    let provider = snapshot.providers[0]
    #expect(DockSummary(provider: provider, now: snapshot.generatedAt).value == "12%")
    let personal = DockSummary(provider: provider, now: snapshot.generatedAt, account: .account("personal"))
    #expect(personal.value == "71%" && personal.isCached && personal.currentPercent == nil)
    for id in ["failed", "missing", "Work"] {
        let unavailable = DockSummary(provider: provider, now: snapshot.generatedAt, account: .account(id))
        #expect(unavailable.value == "—" && unavailable.percent == nil)
    }
    let missing = DockAccountFocus.account("removed")
    #expect(DockAccountFocus.options(for: provider, including: missing).last == missing)
    #expect(missing.title(in: provider) == "Missing")
}

@Test @MainActor func dockAccountIDsRoundTripWithoutConfusingAllOrModelGroups() throws {
    for account in [DockAccountFocus.all, .account("personal"), .account("all"), .account("Work")] {
        #expect(DockAccountFocus(savedValue: account.id) == account)
    }
    #expect(DockAccountFocus(savedValue: nil) == .all)
    #expect(DockAccountFocus(savedValue: "account:") == .all)
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-15T00:00:00Z","providers":[
              {"id":"agy","name":"Antigravity","installed":true,"rows":[{"id":"gemini","name":"Gemini"},{"id":"claude","name":"Claude"}]},
              {"id":"claude","name":"Claude","installed":true,"rows":[{"id":"work","name":"work"}]}
            ]}
            """#.utf8))
    #expect(!DockAccountFocus.all.canChoose(in: snapshot.providers[0]))
    #expect(!DockAccountFocus.all.canChoose(in: snapshot.providers[1]))
    #expect(DockAccountFocus.account("removed").canChoose(in: snapshot.providers[1]))
    #expect(DockAccountFocus.account("work").title(in: snapshot.providers[1]) == "Work")
}

@Test @MainActor func selectedAccountDoesNotInheritAnotherAccountsBlockOrExpiredWindow() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-15T00:00:00Z","providers":[{"id":"codex","name":"Codex","installed":true,"rows":[
              {"id":"personal","name":"Personal","observedAt":"2026-11-15T00:00:00Z","windows":[{"label":"5h","pct":99,"resetsAt":"2026-11-14T23:00:00Z"},{"label":"7d","pct":12}]},
              {"id":"work","name":"Work","observedAt":"2026-11-15T00:00:00Z","windows":[{"label":"5h","pct":100,"limited":true}]}]}]}
            """#.utf8))
    let provider = snapshot.providers[0]
    #expect(DockSummary(provider: provider, now: snapshot.generatedAt).blocked)
    let personal = DockSummary(
        provider: provider, now: snapshot.generatedAt, valueMode: .remaining, account: .account("personal"))
    #expect(!personal.blocked && personal.value == "88%")
    #expect(
        DockSummary(provider: provider, now: snapshot.generatedAt, focus: .window("5h"), account: .account("personal"))
            .value == "—")
}
