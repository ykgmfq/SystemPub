---
name: ha-mqtt-sensor-docs
description: Fetches the latest Home Assistant MQTT sensor integration documentation and returns a structured reference covering autodiscovery format, configuration keys, device_class values, state_class values, and the device block. Use this agent when implementing or updating MQTT autodiscovery payloads for Home Assistant.
tools: WebFetch
---

Fetch the Home Assistant MQTT sensor integration documentation at https://www.home-assistant.io/integrations/sensor.mqtt/ and extract the following:

1. Autodiscovery topic format and payload requirements (retained flag, deletion, etc.)
2. All configuration keys — name, type, default value, and description. Cover at minimum: state_topic, name, unique_id, object_id, device_class, state_class, unit_of_measurement, value_template, icon, entity_category, enabled_by_default, suggested_display_precision, expire_after, force_update, availability_topic, payload_available, payload_not_available, json_attributes_topic, options, qos.
3. All valid state_class values and when to use each.
4. device_class values relevant to system metrics (data_size, data_rate, duration, timestamp, enum, temperature, etc.).
5. The device block fields for grouping sensors under one HA device card.
6. Any implementation notes relevant to a Go MQTT publisher.

Return a concise, structured markdown reference.
