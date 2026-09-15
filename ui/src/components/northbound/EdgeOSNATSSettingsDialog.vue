<template>
  <a-modal
    v-model:visible="visible"
    :title="isEdit ? '编辑 edgeOS (NATS)' : '新增 edgeOS (NATS)'"
    :width="960"
    modal-class="northbound-settings-modal"
    unmount-on-close
    :footer="true"
    :mask-closable="false"
  >
    <div class="nb-mode-banner nb-mode-banner--push">
      <span class="nb-mode-banner__tag">主动上报</span>
      <span>edgeOS 平台 NATS 2.x，JetStream 持久化与高性能消息传递</span>
    </div>

    <a-tabs v-model:active-key="activeTab" type="rounded" size="small">
      <a-tab-pane key="basic">
        <template #title>连接配置</template>
        <a-form :model="form" layout="vertical" class="industrial-form form-controls-md">
          <!-- 通道名称 + 启用 -->
          <div class="nb-channel-header">
            <a-form-item label="通道名称" required class="nb-channel-name-item">
              <a-input v-model="form.name" placeholder="例如: edgeOS NATS 生产通道" />
            </a-form-item>
            <a-form-item label="启用" class="nb-enable-item">
              <a-switch v-model="form.enable" />
            </a-form-item>
          </div>

          <div class="nb-form-section">
            <div class="nb-form-section__title">NATS 连接</div>
            <a-form-item label="服务器 URL" required>
              <a-input v-model="form.url" placeholder="nats://127.0.0.1:4222" class="mono-text" />
            </a-form-item>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="Client ID" required>
                  <a-input v-model="form.client_id" placeholder="edgeCore-node-001" class="mono-text" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="节点 ID" required>
                  <a-input v-model="form.node_id" placeholder="edgeCore-node-001" class="mono-text" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="用户名"><a-input v-model="form.username" placeholder="可选" /></a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="密码"><a-input-password v-model="form.password" placeholder="可选" /></a-form-item>
              </a-col>
            </a-row>
            <a-form-item label="Token">
              <a-input v-model="form.token" placeholder="可选" class="mono-text" />
            </a-form-item>
          </div>

          <div class="nb-form-section">
            <div class="nb-form-section__title">连接选项</div>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="JetStream"><a-switch v-model="form.jetstream_enabled" /></a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="心跳周期">
                  <a-input v-model="form.heartbeat_interval" placeholder="30s" class="mono-text" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="连接超时 (秒)">
                  <a-input-number v-model="form.connect_timeout" :min="1" :max="300" placeholder="30" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="重连间隔 (秒)">
                  <a-input-number v-model="form.reconnect_wait" :min="1" :max="60" placeholder="2" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="最大重连次数">
                  <a-input-number v-model="form.max_reconnects" :min="1" :max="100" placeholder="10" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-form-item label="Ping 间隔 (秒)">
              <a-input-number v-model="form.ping_interval" :min="1" :max="300" placeholder="20" />
            </a-form-item>
          </div>
        </a-form>
      </a-tab-pane>

      <a-tab-pane key="real-devices">
        <template #title>上报真实设备</template>
        <NorthboundReportStrategyPanel
          device-kind="real"
          :visible="visible"
          :all-devices="localDevices.length ? localDevices : allDevices"
          v-model:devices="form.devices"
          v-model:virtual-devices="form.virtual_devices"
        />
      </a-tab-pane>

      <a-tab-pane key="virtual-devices">
        <template #title>上报虚拟设备</template>
        <NorthboundReportStrategyPanel
          device-kind="virtual"
          :visible="visible"
          :all-devices="localDevices.length ? localDevices : allDevices"
          v-model:devices="form.devices"
          v-model:virtual-devices="form.virtual_devices"
        />
      </a-tab-pane>

      <!-- EAN 2.0 能力层 / EAN Capability Layer -->
      <a-tab-pane key="ean">
        <template #title>EAN 能力层</template>
        <div class="ean-capability-panel">
          <!-- 页头 -->
          <div class="ean-panel-header">
            <div class="ean-panel-header__icon">
              <icon-robot />
            </div>
            <div class="ean-panel-header__meta">
              <div class="ean-panel-header__title">EAN 2.0 能力层</div>
              <div class="ean-panel-header__sub">Edge Agent Network · 基于此通道承载 Agent 通信</div>
            </div>
            <a-tag :color="form.ean_enabled ? 'green' : 'gray'" size="small" class="ean-panel-header__badge">
              {{ form.ean_enabled ? '已启用' : '未启用' }}
            </a-tag>
          </div>

          <!-- 配置卡片列表 -->
          <div class="ean-config-cards">
            <!-- 卡片 1：启用开关 -->
            <div class="ean-config-card" :class="{ 'is-active': form.ean_enabled }">
              <div class="ean-config-card__left">
                <div class="ean-config-card__icon ean-icon--enable">
                  <icon-thunderbolt />
                </div>
                <div class="ean-config-card__text">
                  <div class="ean-config-card__label">启用 EAN 能力层</div>
                  <div class="ean-config-card__desc">启用后此通道将承载 EAN 2.0 Agent 注册、能力发现和远程调用</div>
                </div>
              </div>
              <a-switch v-model="form.ean_enabled" />
            </div>

            <!-- 卡片 2：心跳间隔 -->
            <div class="ean-config-card" :class="{ 'is-disabled': !form.ean_enabled }">
              <div class="ean-config-card__left">
                <div class="ean-config-card__icon ean-icon--heartbeat">
                  <icon-pulse />
                </div>
                <div class="ean-config-card__text">
                  <div class="ean-config-card__label">心跳间隔</div>
                  <div class="ean-config-card__desc">Agent 向 EdgeOS 发送心跳的间隔，默认 60 秒</div>
                </div>
              </div>
              <div class="ean-config-card__control">
                <a-input-number
                  v-model="form.ean_heartbeat_sec"
                  :min="10" :max="600" :step="10"
                  :disabled="!form.ean_enabled"
                  style="width: 96px"
                />
                <span class="ean-unit">秒</span>
              </div>
            </div>

            <!-- 卡片 3：事件自动发布 -->
            <div class="ean-config-card" :class="{ 'is-disabled': !form.ean_enabled }">
              <div class="ean-config-card__left">
                <div class="ean-config-card__icon ean-icon--event">
                  <icon-share-alt />
                </div>
                <div class="ean-config-card__text">
                  <div class="ean-config-card__label">事件自动发布</div>
                  <div class="ean-config-card__desc">设备数据变化时自动发布 EAN Event</div>
                </div>
              </div>
              <a-switch v-model="form.ean_event_auto_publish" :disabled="!form.ean_enabled" />
            </div>
          </div>
        </div>
      </a-tab-pane>
    </a-tabs>

    <template #footer>
      <div class="industrial-modal-footer">
        <a-button @click="visible = false">取消</a-button>
        <a-button type="primary" :loading="loading" @click="saveSettings">保存</a-button>
      </div>
    </template>
  </a-modal>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import request from '@/utils/request'
import { fetchAllSouthboundDevices } from '@/utils/southboundDevices'
import {
  closeNorthboundSettingsDialog,
  extractNorthboundSaveWarning,
  northboundSaveRequestConfig,
  notifyNorthboundSaveError,
  notifyNorthboundSaveSuccess,
  notifyNorthboundValidationError,
  validateNorthboundChannelName
} from '@/utils/northboundSave'
import NorthboundReportStrategyPanel from '@/components/northbound/NorthboundReportStrategyPanel.vue'

const props = defineProps({
  visible: { type: Boolean, default: false },
  config: { type: Object, default: null },
  allDevices: { type: Array, default: () => [] },
  northboundConfig: { type: Object, default: () => ({}) }
})

const emit = defineEmits(['update:visible', 'saved'])

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

const loading = ref(false)
const form = ref({})
const activeTab = ref('basic')
const localDevices = ref([])
const isNewMode = ref(false)

const isEdit = computed(() => props.config && props.config.id)

const initForm = () => {
  activeTab.value = 'basic'
  isNewMode.value = !props.config
  if (props.config) {
    form.value = JSON.parse(JSON.stringify(props.config))
  } else {
    form.value = {
      id: 'edgeos-nats_' + Date.now(),
      enable: true,
      name: 'New edgeOS NATS',
      url: 'nats://127.0.0.1:4222',
      client_id: 'edgeCore-node-001',
      node_id: 'edgeCore-node-001',
      username: '',
      password: '',
      token: '',
      connect_timeout: 30,
      reconnect_wait: 2,
      max_reconnects: 10,
      ping_interval: 20,
      jetstream_enabled: false,
      heartbeat_interval: '30s',
      devices: {},
      virtual_devices: {},
      ean_enabled: false,
      ean_heartbeat_sec: 60,
      ean_event_auto_publish: false
    }
  }
  if (!form.value.devices) form.value.devices = {}
  if (!form.value.virtual_devices) form.value.virtual_devices = {}
}

const resolveDevices = async () => {
  if (props.allDevices?.length) {
    localDevices.value = props.allDevices
    return
  }
  try {
    localDevices.value = await fetchAllSouthboundDevices(request)
  } catch (e) {
    console.error('Failed to fetch southbound devices', e)
    localDevices.value = []
  }
}

watch(() => props.visible, async (val) => {
  if (!val) return
  initForm()
  await resolveDevices()
})

watch(() => props.allDevices, (devs) => {
  if (!props.visible || !devs?.length) return
  localDevices.value = devs
}, { deep: true })

const saveSettings = async () => {
  const missing = []
  if (!form.value.name?.trim()) missing.push('通道名称')
  if (!form.value.url?.trim()) missing.push('服务器 URL')
  if (!form.value.client_id?.trim()) missing.push('Client ID')
  if (!form.value.node_id?.trim()) missing.push('节点 ID')
  if (missing.length) {
    notifyNorthboundValidationError('请填写必填项：' + missing.join('、'))
    activeTab.value = 'basic'
    return
  }

  const nameError = validateNorthboundChannelName(form.value.name, form.value.id, props.northboundConfig)
  if (nameError) {
    notifyNorthboundValidationError(nameError)
    activeTab.value = 'basic'
    return
  }

  loading.value = true
  try {
    const res = await request.post('/api/northbound/edgeos-nats', { ...form.value }, northboundSaveRequestConfig)
    notifyNorthboundSaveSuccess('edgeOS (NATS)', isNewMode.value, extractNorthboundSaveWarning(res))
    closeNorthboundSettingsDialog(emit)
    emit('saved')
  } catch (e) {
    notifyNorthboundSaveError(e, 'edgeOS (NATS)')
  } finally {
    loading.value = false
  }
}
</script>
