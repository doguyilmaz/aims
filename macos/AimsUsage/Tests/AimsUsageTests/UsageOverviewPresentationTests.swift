import Testing

@testable import AimsUsage

@Test @MainActor func overviewChangesEntryOnlyAfterTheNativeCloseCallback() {
    var state = UsageOverviewPresentation.hidden
    let menu = UsageOverviewRequest(source: .menuBar, pinned: false)
    let dock = UsageOverviewRequest(source: .dock, pinned: false)
    state.toggle(from: .menuBar)
    #expect(state == .presented(menu))
    state.toggle(from: .dock)
    #expect(state == .closing(current: menu, next: dock))
    state.willClose()
    #expect(state == .closing(current: menu, next: dock))
    state.didClose()
    #expect(state == .presented(dock))
}

@Test @MainActor func pinChangesTheSamePresentationWithoutReopeningIt() {
    var state = UsageOverviewPresentation.presented(UsageOverviewRequest(source: .menuBar, pinned: false))
    state.togglePin()
    #expect(state == .presented(UsageOverviewRequest(source: .menuBar, pinned: true)))
    state.togglePin()
    #expect(state == .presented(UsageOverviewRequest(source: .menuBar, pinned: false)))
}

@Test @MainActor func closingEntryDoesNotReopenFromTheSameMouseGesture() {
    var state = UsageOverviewPresentation.presented(UsageOverviewRequest(source: .menuBar, pinned: false))
    state.willClose()
    state.toggle(from: .menuBar)
    state.didClose()
    #expect(state == .hidden)
}

@Test @MainActor func pinSurvivesEntryChangeAndDockStaysVisibleUntilNativeCloseFinishes() {
    var state = UsageOverviewPresentation.presented(UsageOverviewRequest(source: .dock, pinned: true))
    state.toggle(from: .menuBar)
    #expect(state.holdsDockOpen)
    state.didClose()
    #expect(state == .presented(UsageOverviewRequest(source: .menuBar, pinned: true)))
    #expect(!state.holdsDockOpen)
}

@Test @MainActor func explicitCloseCancelsAQueuedOverviewReopen() {
    var state = UsageOverviewPresentation.presented(UsageOverviewRequest(source: .menuBar, pinned: false))
    state.toggle(from: .dock)
    state.close()
    state.didClose()
    #expect(state == .hidden)
}

@Test @MainActor func nativeDismissalEndsPresentationAndSavedPinIntentCanBeRestored() {
    var state = UsageOverviewPresentation.hidden
    state.toggle(from: .dock, pinnedWhenOpening: true)
    #expect(state.isPinned)
    state.willClose()
    state.didClose()
    #expect(!state.isVisible)
    state.toggle(from: .menuBar)
    #expect(state.isVisible && !state.isPinned)
}

@Test @MainActor func showRaisesTheExistingOverviewAndCanQueueAnExplicitReopen() {
    let menu = UsageOverviewRequest(source: .menuBar, pinned: true)
    var state = UsageOverviewPresentation.presented(menu)
    state.show(from: .menuBar)
    #expect(state == .presented(menu))
    state.close()
    state.show(from: .menuBar)
    #expect(state == .closing(current: menu, next: menu))
    state.didClose()
    #expect(state == .presented(menu))
}
