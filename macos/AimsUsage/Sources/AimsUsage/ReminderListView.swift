import SwiftUI

struct ReminderListView: View {
    @Bindable var model: UsageModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                Text("Your reminders").font(.system(size: 14, weight: .semibold))
                ForEach(model.snapshot?.customResets ?? []) { reminder in
                    HStack(alignment: .top) {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(reminder.label).font(.system(size: 12, weight: .medium))
                            Text(
                                "\(reminder.provider) / \(reminder.row) · \(reminder.at.formatted(date: .abbreviated, time: .shortened))"
                            )
                            .font(.system(size: 11)).foregroundStyle(.secondary)
                        }
                        Spacer(minLength: 0)
                        Button {
                            Task { await model.removeReminder(id: reminder.id) }
                        } label: {
                            Image(systemName: "trash")
                        }
                        .buttonStyle(.borderless).disabled(model.isRefreshing || model.fixtureURL != nil)
                        .accessibilityLabel("Remove reminder \(reminder.label)")
                    }
                }
                Text("Personal reminders do not change provider quota.").font(.system(size: 11)).foregroundStyle(
                    .secondary)
            }.padding(16)
        }.frame(maxHeight: 360)
    }
}
