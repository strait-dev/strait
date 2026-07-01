import { Section, Text } from "@react-email/components";
import { NotificationLayout, NotificationText } from "./notification-layout";

type GenericNotificationProps = {
  eventType: string;
  payload: string;
};

const GenericNotification = ({
  eventType,
  payload,
}: GenericNotificationProps) => (
  <NotificationLayout heading="Strait notification" preview={eventType}>
    <NotificationText>
      Event: <strong style={{ color: "#252525" }}>{eventType}</strong>
    </NotificationText>

    <br />

    <Section>
      <Text className="m-0 whitespace-pre-wrap rounded-[0.1rem] bg-[#F7F7F7] p-4 font-mono text-[#252525] text-xs leading-5">
        {payload}
      </Text>
    </Section>
  </NotificationLayout>
);

GenericNotification.PreviewProps = {
  eventType: "usage.forecast_warning",
  payload: '{ "org_id": "org_123" }',
};

export default GenericNotification;
